package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	authmw "life-dashboard/internal/middleware"
)

const (
	inputJobMaxAttempts = 3
	inputJobLease       = 7 * time.Minute
	inputJobPoll        = 2 * time.Second
	// The extraction model normally answers in seconds. Waiting five minutes for
	// a connection that has stopped producing bytes only delays the useful retry.
	inputJobAttemptBudget  = 90 * time.Second
	inputNotificationLease = time.Minute
)

var inputJobBackoff = [...]time.Duration{15 * time.Second, time.Minute, 5 * time.Minute}

type inputJobAccepted struct {
	JobID   string `json:"job_id"`
	Status  string `json:"status"`
	Display string `json:"display"`
}

type inputJob struct {
	ID              string
	UserID          string
	RawEventID      string
	Text            string
	Finish          bool
	DurationMinutes int
	Typed           bool
	Attempts        int
}

type inputNotificationJob struct {
	ID       string
	UserID   string
	Attempts int
	Result   voiceWorkoutResponse
}

type inputJobView struct {
	ID          string                `json:"id"`
	Text        string                `json:"text"`
	Status      string                `json:"status"`
	Attempts    int                   `json:"attempts"`
	Result      *voiceWorkoutResponse `json:"result,omitempty"`
	LastError   string                `json:"last_error,omitempty"`
	CreatedAt   time.Time             `json:"created_at"`
	CompletedAt *time.Time            `json:"completed_at,omitempty"`
}

func (h *VoiceWorkoutHandler) enqueueText(w http.ResponseWriter, r *http.Request, userID string, envelope voiceWorkoutEnvelope, typed bool) {
	text := normalizeVoiceText(envelope.Text)
	if !voiceHasContent(text) {
		display := "Ничего не услышал. Повтори."
		if typed {
			display = "Нечего отправлять. Введи текст."
		}
		writeJSONStatus(w, http.StatusOK, voiceWorkoutResponse{
			typed: typed, Status: "ok", Domain: voiceDomainUnknown, Heard: text, Display: display,
		})
		return
	}

	envelope.Text = text
	jobID, err := h.insertInputJob(r.Context(), userID, envelope, typed)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("enqueue input")
		http.Error(w, "cannot queue input", http.StatusInternalServerError)
		return
	}

	select {
	case h.wake <- struct{}{}:
	default:
	}
	writeJSONStatus(w, http.StatusAccepted, inputJobAccepted{
		JobID: jobID, Status: "queued", Display: "Принял. Обработаю в фоне и пришлю уведомление.",
	})
}

func (h *VoiceWorkoutHandler) insertInputJob(ctx context.Context, userID string, envelope voiceWorkoutEnvelope, typed bool) (string, error) {
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	// Deliberately reconstruct the archived payload: the public request also
	// contains api_key, and credentials must never be copied into raw_events.
	payload, err := json.Marshal(map[string]any{
		"text": envelope.Text, "finish": envelope.Finish,
		"duration_minutes": envelope.DurationMinutes, "typed": typed,
	})
	if err != nil {
		return "", err
	}

	var eventID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO raw_events (source, event_type, payload, user_id)
		VALUES ('voice', 'phrase', $1::jsonb, $2)
		RETURNING id
	`, payload, userID).Scan(&eventID); err != nil {
		return "", err
	}

	var jobID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO input_jobs (user_id, raw_event_id, text, finish, duration_minutes, typed)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, userID, eventID, envelope.Text, envelope.Finish, envelope.DurationMinutes, typed).Scan(&jobID); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return jobID, nil
}

// StartInputWorker starts one ordered consumer. A single consumer is deliberate:
// workout phrases are stateful, and processing two phrases from one user at the
// same time can corrupt the accumulated draft.
func (h *VoiceWorkoutHandler) StartInputWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(inputJobPoll)
		defer ticker.Stop()
		for {
			for h.processNextInputJob(ctx) {
			}
			for h.processNextInputNotification(ctx) {
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

func (h *VoiceWorkoutHandler) processNextInputJob(workerCtx context.Context) bool {
	job, err := h.claimInputJob(workerCtx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("claim input job")
		return false
	}

	ctx, cancel := context.WithTimeout(workerCtx, inputJobAttemptBudget)
	response, processErr := h.processText(ctx, job.UserID, job.RawEventID, voiceWorkoutEnvelope{
		Text: job.Text, Finish: job.Finish, DurationMinutes: job.DurationMinutes,
	}, job.Typed)
	cancel()

	if processErr == nil {
		if err := h.completeInputJob(workerCtx, job.ID, response); err != nil {
			h.logger.Error().Err(err).Str("job_id", job.ID).Msg("complete input job")
			return true
		}
		return true
	}

	retryable := response.ParseError != "" || errors.Is(processErr, context.DeadlineExceeded)
	if retryable && job.Attempts < inputJobMaxAttempts {
		delay := inputJobBackoff[job.Attempts-1]
		if err := h.retryInputJob(workerCtx, job.ID, processErr.Error(), delay); err != nil {
			h.logger.Error().Err(err).Str("job_id", job.ID).Msg("retry input job")
		}
		return true
	}

	response.Status = "failed"
	if response.Display == "" {
		response.Display = "Не удалось обработать: " + processErr.Error()
	}
	if err := h.failInputJob(workerCtx, job.ID, response, processErr.Error()); err != nil {
		h.logger.Error().Err(err).Str("job_id", job.ID).Msg("fail input job")
		return true
	}
	return true
}

func (h *VoiceWorkoutHandler) processNextInputNotification(ctx context.Context) bool {
	job, err := h.claimInputNotification(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("claim input notification")
		return false
	}

	success := job.Result.Status != "failed"
	if err := h.push.sendInputResult(ctx, job.UserID, job.ID, job.Result.Display, success); err != nil {
		delay := time.Duration(job.Attempts) * time.Minute
		if delay > time.Hour {
			delay = time.Hour
		}
		if updateErr := h.retryInputNotification(ctx, job.ID, err.Error(), delay); updateErr != nil {
			h.logger.Error().Err(updateErr).Str("job_id", job.ID).Msg("retry input notification")
		}
		return true
	}
	if _, err := h.db.Exec(ctx, `
		UPDATE input_jobs
		SET notification_status = 'sent', notification_sent_at = NOW(),
		    notification_error = NULL, updated_at = NOW()
		WHERE id = $1
	`, job.ID); err != nil {
		h.logger.Error().Err(err).Str("job_id", job.ID).Msg("complete input notification")
	}
	return true
}

func (h *VoiceWorkoutHandler) claimInputNotification(ctx context.Context) (inputNotificationJob, error) {
	var job inputNotificationJob
	var raw []byte
	err := h.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM input_jobs
			WHERE status IN ('succeeded', 'failed')
			  AND notification_status IN ('pending', 'sending')
			  AND notification_available_at <= NOW()
			ORDER BY completed_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE input_jobs j
		SET notification_status = 'sending',
		    notification_attempts = notification_attempts + 1,
		    notification_available_at = NOW() + $1::interval,
		    updated_at = NOW()
		FROM candidate
		WHERE j.id = candidate.id
		RETURNING j.id, j.user_id, j.notification_attempts, j.result
	`, inputNotificationLease.String()).Scan(&job.ID, &job.UserID, &job.Attempts, &raw)
	if err != nil {
		return job, err
	}
	if err := json.Unmarshal(raw, &job.Result); err != nil {
		return job, err
	}
	return job, nil
}

func (h *VoiceWorkoutHandler) retryInputNotification(ctx context.Context, jobID, message string, delay time.Duration) error {
	_, err := h.db.Exec(ctx, `
		UPDATE input_jobs
		SET notification_status = 'pending',
		    notification_available_at = NOW() + $2::interval,
		    notification_error = $3, updated_at = NOW()
		WHERE id = $1
	`, jobID, delay.String(), message)
	return err
}

func (h *VoiceWorkoutHandler) claimInputJob(ctx context.Context) (inputJob, error) {
	var job inputJob
	err := h.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT j.id
			FROM input_jobs j
			WHERE ((j.status = 'queued' AND j.available_at <= NOW())
			    OR (j.status = 'processing' AND j.locked_until <= NOW()))
			  AND NOT EXISTS (
				SELECT 1 FROM input_jobs earlier
				WHERE earlier.user_id = j.user_id
				  AND (earlier.created_at, earlier.id) < (j.created_at, j.id)
				  AND earlier.status IN ('queued', 'processing')
			  )
			ORDER BY j.created_at, j.id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE input_jobs j
		SET status = 'processing', attempts = attempts + 1,
		    started_at = COALESCE(started_at, NOW()),
		    locked_until = NOW() + $1::interval, updated_at = NOW()
		FROM candidate
		WHERE j.id = candidate.id
		RETURNING j.id, j.user_id, j.raw_event_id, j.text, j.finish,
		          j.duration_minutes, j.typed, j.attempts
	`, inputJobLease.String()).Scan(
		&job.ID, &job.UserID, &job.RawEventID, &job.Text, &job.Finish,
		&job.DurationMinutes, &job.Typed, &job.Attempts,
	)
	return job, err
}

func (h *VoiceWorkoutHandler) completeInputJob(ctx context.Context, jobID string, response voiceWorkoutResponse) error {
	encoded, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = h.db.Exec(ctx, `
		UPDATE input_jobs
		SET status = 'succeeded', result = $2::jsonb, last_error = NULL,
		    locked_until = NULL, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, jobID, encoded)
	return err
}

func (h *VoiceWorkoutHandler) retryInputJob(ctx context.Context, jobID, message string, delay time.Duration) error {
	_, err := h.db.Exec(ctx, `
		UPDATE input_jobs
		SET status = 'queued', available_at = NOW() + $2::interval,
		    locked_until = NULL, last_error = $3, updated_at = NOW()
		WHERE id = $1
	`, jobID, delay.String(), message)
	return err
}

func (h *VoiceWorkoutHandler) failInputJob(ctx context.Context, jobID string, response voiceWorkoutResponse, message string) error {
	encoded, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = h.db.Exec(ctx, `
		UPDATE input_jobs
		SET status = 'failed', result = $2::jsonb, last_error = $3,
		    locked_until = NULL, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, jobID, encoded, message)
	return err
}

func (h *VoiceWorkoutHandler) ListInputJobs(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(authmw.UserIDKey).(string)
	rows, err := h.db.Query(r.Context(), `
		SELECT id, text, status, attempts, result, COALESCE(last_error, ''), created_at, completed_at
		FROM input_jobs WHERE user_id = $1
		ORDER BY created_at DESC LIMIT 30
	`, userID)
	if err != nil {
		http.Error(w, "cannot load input jobs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	jobs := make([]inputJobView, 0)
	for rows.Next() {
		job, err := scanInputJobView(rows)
		if err != nil {
			http.Error(w, "cannot load input jobs", http.StatusInternalServerError)
			return
		}
		jobs = append(jobs, job)
	}
	writeJSONStatus(w, http.StatusOK, jobs)
}

func (h *VoiceWorkoutHandler) GetInputJob(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(authmw.UserIDKey).(string)
	job, err := scanInputJobView(h.db.QueryRow(r.Context(), `
		SELECT id, text, status, attempts, result, COALESCE(last_error, ''), created_at, completed_at
		FROM input_jobs WHERE id = $1 AND user_id = $2
	`, chi.URLParam(r, "jobID"), userID))
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "input job not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "cannot load input job", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusOK, job)
}

type inputJobScanner interface {
	Scan(dest ...any) error
}

func scanInputJobView(scanner inputJobScanner) (inputJobView, error) {
	var job inputJobView
	var raw []byte
	if err := scanner.Scan(&job.ID, &job.Text, &job.Status, &job.Attempts, &raw,
		&job.LastError, &job.CreatedAt, &job.CompletedAt); err != nil {
		return job, err
	}
	if len(raw) > 0 {
		var result voiceWorkoutResponse
		if err := json.Unmarshal(raw, &result); err != nil {
			return job, err
		}
		job.Result = &result
	}
	return job, nil
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
