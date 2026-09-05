package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	authmw "life-dashboard/internal/middleware"
)

const (
	checkupJobMaxAttempts = 3
	// The lease outlives one attempt so a crashed worker's job is only picked up
	// again once the attempt it was running can no longer be in flight.
	checkupJobLease = 15 * time.Minute
	checkupJobPoll  = 2 * time.Second
	// A checkup on the full context routinely runs past a minute on a reasoning
	// model, so an attempt gets most of the upstream budget rather than the
	// short leash the extraction path uses.
	checkupJobAttemptBudget  = 12 * time.Minute
	checkupNotificationLease = time.Minute
)

// Unlike a dictated phrase, a checkup is worth retrying soon: the user is
// waiting for it, and provider hiccups are usually over in minutes.
var checkupJobBackoff = [...]time.Duration{2 * time.Minute, 10 * time.Minute}

type checkupJobAccepted struct {
	JobID       string `json:"job_id"`
	Status      string `json:"status"`
	Period      string `json:"period"`
	PeriodLabel string `json:"period_label"`
	Message     string `json:"message"`
}

type checkupJob struct {
	ID       string
	UserID   string
	Period   string
	Attempts int
}

type checkupJobView struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	Period      string     `json:"period"`
	PeriodLabel string     `json:"period_label"`
	Attempts    int        `json:"attempts"`
	Content     string     `json:"content,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type checkupNotificationJob struct {
	ID       string
	UserID   string
	Period   string
	Status   string
	Attempts int
	Content  string
}

// Checkup queues a report instead of generating it inside the request.
//
// The report used to be streamed straight back and stored only once the model
// had finished, so a phone that locked its screen halfway through threw away a
// generation that had already been paid for. The job outlives the connection,
// and the client either polls it or gets a push when it lands.
func (h *AIHandler) Checkup(w http.ResponseWriter, r *http.Request) {
	if h.unleash != nil && !h.unleash.IsEnabled("ai-chat") {
		http.Error(w, "AI чат временно отключён", http.StatusForbidden)
		return
	}

	var req CheckupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)

	lastReportAt, err := h.getLastCheckupAt(ctx, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("load last ai checkup")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Resolved here only to reject an unknown period before anything is queued;
	// the worker resolves the window again against the time it actually runs.
	window, err := resolveCheckupWindow(time.Now(), req.Period, lastReportAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	jobID, existing, err := h.enqueueCheckupJob(ctx, userID, window.RequestedPeriod)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("enqueue checkup")
		http.Error(w, "cannot queue checkup", http.StatusInternalServerError)
		return
	}

	select {
	case h.wake <- struct{}{}:
	default:
	}

	message := "Собираю checkup в фоне, пришлю уведомление."
	if existing {
		message = "Checkup уже собирается, дождись его."
	}
	writeJSONStatus(w, http.StatusAccepted, checkupJobAccepted{
		JobID:       jobID,
		Status:      "queued",
		Period:      window.RequestedPeriod,
		PeriodLabel: checkupPeriodLabel(window.RequestedPeriod),
		Message:     message,
	})
}

// enqueueCheckupJob returns the job already in flight rather than starting a
// second one: two checkups running at once cost twice and answer the same.
func (h *AIHandler) enqueueCheckupJob(ctx context.Context, userID, period string) (string, bool, error) {
	var existingID string
	err := h.db.QueryRow(ctx, `
		SELECT id FROM ai_checkup_jobs
		WHERE user_id = $1 AND status IN ('queued', 'processing')
		ORDER BY created_at
		LIMIT 1
	`, userID).Scan(&existingID)
	if err == nil {
		return existingID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}

	var jobID string
	if err := h.db.QueryRow(ctx, `
		INSERT INTO ai_checkup_jobs (user_id, requested_period)
		VALUES ($1, $2)
		RETURNING id
	`, userID, period).Scan(&jobID); err != nil {
		return "", false, err
	}
	return jobID, false, nil
}

// StartCheckupWorker starts the single consumer of queued checkups.
func (h *AIHandler) StartCheckupWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(checkupJobPoll)
		defer ticker.Stop()
		for {
			for h.processNextCheckupJob(ctx) {
			}
			for h.processNextCheckupNotification(ctx) {
			}
			select {
			case <-ctx.Done():
				return
			case <-h.wake:
			case <-ticker.C:
			}
		}
	}()
}

func (h *AIHandler) processNextCheckupJob(workerCtx context.Context) bool {
	job, err := h.claimCheckupJob(workerCtx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("claim checkup job")
		return false
	}

	ctx, cancel := context.WithTimeout(workerCtx, checkupJobAttemptBudget)
	content, generateErr := h.generateCheckup(ctx, job)
	cancel()

	if generateErr == nil {
		if err := h.completeCheckupJob(workerCtx, job.ID, content); err != nil {
			h.logger.Error().Err(err).Str("job_id", job.ID).Msg("complete checkup job")
		}
		return true
	}

	if job.Attempts < checkupJobMaxAttempts {
		delay := checkupJobBackoff[job.Attempts-1]
		h.logger.Warn().Err(generateErr).Str("job_id", job.ID).Dur("retry_in", delay).Msg("checkup attempt failed")
		if err := h.retryCheckupJob(workerCtx, job.ID, generateErr.Error(), delay); err != nil {
			h.logger.Error().Err(err).Str("job_id", job.ID).Msg("retry checkup job")
		}
		return true
	}

	h.logger.Error().Err(generateErr).Str("job_id", job.ID).Msg("checkup job failed")
	if err := h.failCheckupJob(workerCtx, job.ID, generateErr.Error()); err != nil {
		h.logger.Error().Err(err).Str("job_id", job.ID).Msg("fail checkup job")
	}
	return true
}

// generateCheckup is the old request-time path, minus the streaming: build the
// context, ask the model, keep the report and the chat exchange.
func (h *AIHandler) generateCheckup(ctx context.Context, job checkupJob) (string, error) {
	now := time.Now()

	lastReportAt, err := h.getLastCheckupAt(ctx, job.UserID)
	if err != nil {
		return "", fmt.Errorf("load last checkup: %w", err)
	}

	window, err := resolveCheckupWindow(now, job.Period, lastReportAt)
	if err != nil {
		return "", err
	}

	dataContext, err := h.buildCheckupContext(ctx, job.UserID, window)
	if err != nil {
		// A missing section is worth reporting on anyway: the whole point of the
		// checkup is what the data says, and saying "no data" beats saying nothing.
		h.logger.Error().Err(err).Str("job_id", job.ID).Msg("build ai checkup context")
		dataContext = "Данные пользователя временно недоступны."
	}

	userPrompt := fmt.Sprintf("Сделай checkup %s.", window.UserLabel)
	content, err := h.complete(ctx, "checkup", []ChatMessage{
		{Role: "system", Content: buildAICheckupPrompt(now, window, dataContext)},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("checkup came back empty")
	}

	// Stored on the worker's context, not the attempt's: the report exists and
	// must not be lost to a budget that expired between the answer and the write.
	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := h.storeCheckupReport(storeCtx, job.UserID, window, content); err != nil {
		h.logger.Warn().Err(err).Str("job_id", job.ID).Msg("store ai checkup report")
	}
	h.storeChatExchange(storeCtx, job.UserID, userPrompt, content)

	return content, nil
}

func (h *AIHandler) claimCheckupJob(ctx context.Context) (checkupJob, error) {
	var job checkupJob
	err := h.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM ai_checkup_jobs
			WHERE (status = 'queued' AND available_at <= NOW())
			   OR (status = 'processing' AND locked_until <= NOW())
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE ai_checkup_jobs j
		SET status = 'processing', attempts = attempts + 1,
		    started_at = COALESCE(started_at, NOW()),
		    locked_until = NOW() + $1::interval, updated_at = NOW()
		FROM candidate
		WHERE j.id = candidate.id
		RETURNING j.id, j.user_id, j.requested_period, j.attempts
	`, checkupJobLease.String()).Scan(&job.ID, &job.UserID, &job.Period, &job.Attempts)
	return job, err
}

func (h *AIHandler) completeCheckupJob(ctx context.Context, jobID, content string) error {
	_, err := h.db.Exec(ctx, `
		UPDATE ai_checkup_jobs
		SET status = 'succeeded', content = $2, last_error = NULL,
		    locked_until = NULL, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, jobID, content)
	return err
}

func (h *AIHandler) retryCheckupJob(ctx context.Context, jobID, message string, delay time.Duration) error {
	_, err := h.db.Exec(ctx, `
		UPDATE ai_checkup_jobs
		SET status = 'queued', available_at = NOW() + $2::interval,
		    locked_until = NULL, last_error = $3, updated_at = NOW()
		WHERE id = $1
	`, jobID, delay.String(), message)
	return err
}

func (h *AIHandler) failCheckupJob(ctx context.Context, jobID, message string) error {
	_, err := h.db.Exec(ctx, `
		UPDATE ai_checkup_jobs
		SET status = 'failed', last_error = $2,
		    locked_until = NULL, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, jobID, message)
	return err
}

func (h *AIHandler) processNextCheckupNotification(ctx context.Context) bool {
	job, err := h.claimCheckupNotification(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("claim checkup notification")
		return false
	}

	success := job.Status == "succeeded"
	body := checkupNotificationBody(job.Period, job.Content, success)

	// Telegram gets the whole report, the push only a line: one is read in a
	// chat, the other on a lock screen. A failure here retries both, and the
	// push carries a per-job tag so the repeat replaces rather than stacks.
	if success && h.telegram != nil {
		if _, err := h.telegram.SendReport(ctx, job.UserID, "Checkup "+checkupPeriodLabel(job.Period), job.Content); err != nil {
			delay := time.Duration(job.Attempts) * time.Minute
			if delay > time.Hour {
				delay = time.Hour
			}
			if updateErr := h.retryCheckupNotification(ctx, job.ID, err.Error(), delay); updateErr != nil {
				h.logger.Error().Err(updateErr).Str("job_id", job.ID).Msg("retry checkup notification")
			}
			h.logger.Warn().Err(err).Str("job_id", job.ID).Msg("send checkup to telegram")
			return true
		}
	}

	if err := h.push.sendCheckupResult(ctx, job.UserID, job.ID, body, success); err != nil {
		delay := time.Duration(job.Attempts) * time.Minute
		if delay > time.Hour {
			delay = time.Hour
		}
		if updateErr := h.retryCheckupNotification(ctx, job.ID, err.Error(), delay); updateErr != nil {
			h.logger.Error().Err(updateErr).Str("job_id", job.ID).Msg("retry checkup notification")
		}
		return true
	}

	if _, err := h.db.Exec(ctx, `
		UPDATE ai_checkup_jobs
		SET notification_status = 'sent', notification_sent_at = NOW(),
		    notification_error = NULL, updated_at = NOW()
		WHERE id = $1
	`, job.ID); err != nil {
		h.logger.Error().Err(err).Str("job_id", job.ID).Msg("complete checkup notification")
	}
	return true
}

// checkupNotificationBody keeps the notification to the first thing the report
// says: the whole point is to pull someone back into the app, not to deliver a
// page of analysis into a lock screen.
func checkupNotificationBody(period, content string, success bool) string {
	label := checkupPeriodLabel(period)
	if !success {
		return label + ": не удалось собрать, попробуй ещё раз."
	}

	summary := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#*->• "))
		if trimmed != "" {
			summary = trimmed
			break
		}
	}
	if summary == "" {
		return label + " готов."
	}
	if len([]rune(summary)) > 140 {
		summary = string([]rune(summary)[:139]) + "…"
	}
	return summary
}

func (h *AIHandler) claimCheckupNotification(ctx context.Context) (checkupNotificationJob, error) {
	var job checkupNotificationJob
	var content *string
	err := h.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM ai_checkup_jobs
			WHERE status IN ('succeeded', 'failed')
			  AND notification_status IN ('pending', 'sending')
			  AND notification_available_at <= NOW()
			ORDER BY completed_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE ai_checkup_jobs j
		SET notification_status = 'sending',
		    notification_attempts = notification_attempts + 1,
		    notification_available_at = NOW() + $1::interval,
		    updated_at = NOW()
		FROM candidate
		WHERE j.id = candidate.id
		RETURNING j.id, j.user_id, j.requested_period, j.status,
		          j.notification_attempts, j.content
	`, checkupNotificationLease.String()).Scan(
		&job.ID, &job.UserID, &job.Period, &job.Status, &job.Attempts, &content,
	)
	if err != nil {
		return job, err
	}
	if content != nil {
		job.Content = *content
	}
	return job, nil
}

func (h *AIHandler) retryCheckupNotification(ctx context.Context, jobID, message string, delay time.Duration) error {
	_, err := h.db.Exec(ctx, `
		UPDATE ai_checkup_jobs
		SET notification_status = 'pending',
		    notification_available_at = NOW() + $2::interval,
		    notification_error = $3, updated_at = NOW()
		WHERE id = $1
	`, jobID, delay.String(), message)
	return err
}

// ListCheckupJobs answers what the page needs on load: whether a checkup is
// still running, and how the last few ended.
func (h *AIHandler) ListCheckupJobs(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(authmw.UserIDKey).(string)
	rows, err := h.db.Query(r.Context(), `
		SELECT id, status, requested_period, attempts, COALESCE(content, ''),
		       COALESCE(last_error, ''), created_at, completed_at
		FROM ai_checkup_jobs
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 10
	`, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("load checkup jobs")
		http.Error(w, "cannot load checkup jobs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	jobs := make([]checkupJobView, 0, 10)
	for rows.Next() {
		job, err := scanCheckupJobView(rows)
		if err != nil {
			h.logger.Error().Err(err).Msg("scan checkup jobs")
			http.Error(w, "cannot load checkup jobs", http.StatusInternalServerError)
			return
		}
		jobs = append(jobs, job)
	}
	writeJSONStatus(w, http.StatusOK, jobs)
}

func (h *AIHandler) GetCheckupJob(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(authmw.UserIDKey).(string)
	job, err := scanCheckupJobView(h.db.QueryRow(r.Context(), `
		SELECT id, status, requested_period, attempts, COALESCE(content, ''),
		       COALESCE(last_error, ''), created_at, completed_at
		FROM ai_checkup_jobs
		WHERE id = $1 AND user_id = $2
	`, chi.URLParam(r, "jobID"), userID))
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "checkup job not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("load checkup job")
		http.Error(w, "cannot load checkup job", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusOK, job)
}

func scanCheckupJobView(scanner inputJobScanner) (checkupJobView, error) {
	var job checkupJobView
	if err := scanner.Scan(&job.ID, &job.Status, &job.Period, &job.Attempts,
		&job.Content, &job.LastError, &job.CreatedAt, &job.CompletedAt); err != nil {
		return job, err
	}
	job.PeriodLabel = checkupPeriodLabel(job.Period)
	return job, nil
}
