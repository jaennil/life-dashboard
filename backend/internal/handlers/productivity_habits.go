package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	authmw "life-dashboard/internal/middleware"
)

const (
	manualHabitSource           = "manual"
	manualHabitWindowDays       = 7
	manualHabitStreakWindowDays = 30
)

type ProductivityHabitsResponse struct {
	Date    string                    `json:"date"`
	Summary ProductivityHabitsSummary `json:"summary"`
	Habits  []ProductivityHabit       `json:"habits"`
}

type ProductivityHabitsSummary struct {
	Total               int     `json:"total"`
	CompletedToday      int     `json:"completed_today"`
	PendingToday        int     `json:"pending_today"`
	MorningPending      int     `json:"morning_pending"`
	EveningPending      int     `json:"evening_pending"`
	AnytimePending      int     `json:"anytime_pending"`
	CompletionRate7Days float64 `json:"completion_rate_7_days"`
}

type ProductivityHabit struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	AreaName        string     `json:"area_name"`
	Routine         string     `json:"routine"`
	Status          string     `json:"status"`
	Completed7Days  int        `json:"completed_7_days"`
	CurrentStreak   int        `json:"current_streak"`
	LastCompletedAt *time.Time `json:"last_completed_at,omitempty"`
}

type saveManualHabitRequest struct {
	Name     string `json:"name"`
	Routine  string `json:"routine"`
	AreaName string `json:"area_name"`
}

type setManualHabitStatusRequest struct {
	Status string `json:"status"`
	Date   string `json:"date"`
}

type manualHabitRow struct {
	ID        string
	Name      string
	AreaName  string
	TimeOfDay []string
}

type manualHabitStatusRow struct {
	HabitID    string
	TargetDate time.Time
	Status     string
}

func (h *ProductivityHandler) GetHabits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)
	targetDate := parseManualHabitDate(r.URL.Query().Get("date"))

	response, err := h.loadManualHabits(ctx, userID, targetDate)
	if err != nil {
		h.logger.Error().Err(err).Msg("load manual habits")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *ProductivityHandler) CreateHabit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)

	var req saveManualHabitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	name, routine, areaName, err := normalizeManualHabitInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rawPayload, _ := json.Marshal(map[string]any{
		"source":    manualHabitSource,
		"routine":   routine,
		"area_name": areaName,
	})

	_, err = h.db.Exec(ctx, `
		INSERT INTO habits (
			user_id, source, external_id, name, area_name, archived,
			recurrence, log_method, time_of_day, remind_at, goal, goal_history_items,
			raw_payload, source_created_at
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), FALSE, 'daily', 'checkbox', $6, $7, $8, NULL, $9, NOW())
	`, userID, manualHabitSource, newManualHabitExternalID(), name, areaName, []string{routine}, []string{}, []byte(`{"kind":"binary"}`), rawPayload)
	if err != nil {
		h.logger.Error().Err(err).Msg("create manual habit")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *ProductivityHandler) UpdateHabit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)
	habitID := strings.TrimSpace(chi.URLParam(r, "habitID"))
	if habitID == "" {
		http.Error(w, "missing habit id", http.StatusBadRequest)
		return
	}

	var req saveManualHabitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	name, routine, areaName, err := normalizeManualHabitInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rawPayload, _ := json.Marshal(map[string]any{
		"source":    manualHabitSource,
		"routine":   routine,
		"area_name": areaName,
	})

	tag, err := h.db.Exec(ctx, `
		UPDATE habits
		SET
			name = $3,
			area_name = NULLIF($4, ''),
			time_of_day = $5,
			raw_payload = $6,
			archived = FALSE
		WHERE id = $1
			AND user_id = $2
			AND source = $7
	`, habitID, userID, name, areaName, []string{routine}, rawPayload, manualHabitSource)
	if err != nil {
		h.logger.Error().Err(err).Msg("update manual habit")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "habit not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ProductivityHandler) DeleteHabit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)
	habitID := strings.TrimSpace(chi.URLParam(r, "habitID"))
	if habitID == "" {
		http.Error(w, "missing habit id", http.StatusBadRequest)
		return
	}

	tag, err := h.db.Exec(ctx, `
		UPDATE habits
		SET archived = TRUE
		WHERE id = $1
			AND user_id = $2
			AND source = $3
	`, habitID, userID, manualHabitSource)
	if err != nil {
		h.logger.Error().Err(err).Msg("archive manual habit")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "habit not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ProductivityHandler) SetHabitStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)
	habitID := strings.TrimSpace(chi.URLParam(r, "habitID"))
	if habitID == "" {
		http.Error(w, "missing habit id", http.StatusBadRequest)
		return
	}

	var req setManualHabitStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	status := normalizeManualHabitStatus(req.Status)
	if status == "" {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	targetDate := parseManualHabitDate(req.Date)

	var exists bool
	if err := h.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM habits
			WHERE id = $1 AND user_id = $2 AND source = $3 AND archived = FALSE
		)
	`, habitID, userID, manualHabitSource).Scan(&exists); err != nil {
		h.logger.Error().Err(err).Msg("check manual habit existence")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "habit not found", http.StatusNotFound)
		return
	}

	currentValue := 0.0
	if status == "completed" {
		currentValue = 1
	}

	rawPayload, _ := json.Marshal(map[string]any{
		"source":      manualHabitSource,
		"status":      status,
		"target_date": targetDate.Format("2006-01-02"),
		"updated_at":  time.Now().UTC().Format(time.RFC3339),
	})

	_, err := h.db.Exec(ctx, `
		INSERT INTO habit_daily_statuses (
			habit_id, target_date, status, current_value, target_value, unit_type, periodicity, raw_payload
		)
		VALUES ($1, $2, $3, $4, 1, 'checkbox', 'daily', $5)
		ON CONFLICT (habit_id, target_date) DO UPDATE SET
			status = EXCLUDED.status,
			current_value = EXCLUDED.current_value,
			target_value = EXCLUDED.target_value,
			unit_type = EXCLUDED.unit_type,
			periodicity = EXCLUDED.periodicity,
			raw_payload = EXCLUDED.raw_payload
	`, habitID, targetDate.Format("2006-01-02"), status, currentValue, rawPayload)
	if err != nil {
		h.logger.Error().Err(err).Msg("set manual habit status")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ProductivityHandler) loadManualHabits(ctx context.Context, userID string, targetDate time.Time) (*ProductivityHabitsResponse, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, name, COALESCE(area_name, ''), COALESCE(time_of_day, ARRAY[]::text[])
		FROM habits
		WHERE user_id = $1
			AND source = $2
			AND archived = FALSE
		ORDER BY
			CASE
				WHEN 'morning' = ANY(COALESCE(time_of_day, ARRAY[]::text[])) THEN 0
				WHEN 'evening' = ANY(COALESCE(time_of_day, ARRAY[]::text[])) THEN 1
				WHEN 'anytime' = ANY(COALESCE(time_of_day, ARRAY[]::text[])) THEN 2
				ELSE 3
			END,
			COALESCE(area_name, ''),
			name
	`, userID, manualHabitSource)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	habits := make([]manualHabitRow, 0)
	habitIDs := make([]string, 0)
	for rows.Next() {
		var row manualHabitRow
		if err := rows.Scan(&row.ID, &row.Name, &row.AreaName, &row.TimeOfDay); err != nil {
			continue
		}
		habits = append(habits, row)
		habitIDs = append(habitIDs, row.ID)
	}

	response := &ProductivityHabitsResponse{
		Date:   targetDate.Format("2006-01-02"),
		Habits: []ProductivityHabit{},
	}
	if len(habits) == 0 {
		return response, nil
	}

	startDate := targetDate.AddDate(0, 0, -(manualHabitStreakWindowDays - 1))
	statusRows, err := h.db.Query(ctx, `
		SELECT s.habit_id, s.target_date, COALESCE(s.status, 'none')
		FROM habit_daily_statuses s
		WHERE s.habit_id = ANY($1)
			AND s.target_date >= $2::date
			AND s.target_date <= $3::date
	`, habitIDs, startDate.Format("2006-01-02"), targetDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer statusRows.Close()

	statusMap := make(map[string]map[string]string, len(habits))
	for statusRows.Next() {
		var row manualHabitStatusRow
		if err := statusRows.Scan(&row.HabitID, &row.TargetDate, &row.Status); err != nil {
			continue
		}
		if _, ok := statusMap[row.HabitID]; !ok {
			statusMap[row.HabitID] = make(map[string]string)
		}
		statusMap[row.HabitID][row.TargetDate.Format("2006-01-02")] = normalizeManualHabitStatus(row.Status)
	}

	weekStart := targetDate.AddDate(0, 0, -(manualHabitWindowDays - 1))
	totalSlots := len(habits) * manualHabitWindowDays
	completedSlots := 0

	for _, habit := range habits {
		statuses := statusMap[habit.ID]
		routine := manualHabitRoutine(habit.TimeOfDay)
		statusToday := statusForDay(statuses, targetDate)
		completed7Days := 0
		var lastCompletedAt *time.Time

		for current := weekStart; !current.After(targetDate); current = current.AddDate(0, 0, 1) {
			if statusForDay(statuses, current) == "completed" {
				completed7Days++
				completedSlots++
			}
		}

		for rawDate, status := range statuses {
			if status != "completed" {
				continue
			}
			parsed, err := time.Parse("2006-01-02", rawDate)
			if err != nil {
				continue
			}
			if lastCompletedAt == nil || parsed.After(*lastCompletedAt) {
				date := parsed
				lastCompletedAt = &date
			}
		}

		response.Habits = append(response.Habits, ProductivityHabit{
			ID:              habit.ID,
			Name:            habit.Name,
			AreaName:        habit.AreaName,
			Routine:         routine,
			Status:          statusToday,
			Completed7Days:  completed7Days,
			CurrentStreak:   completedStreak(statuses, targetDate),
			LastCompletedAt: lastCompletedAt,
		})

		response.Summary.Total++
		if statusToday == "completed" {
			response.Summary.CompletedToday++
		} else {
			response.Summary.PendingToday++
			switch routine {
			case "morning":
				response.Summary.MorningPending++
			case "evening":
				response.Summary.EveningPending++
			default:
				response.Summary.AnytimePending++
			}
		}
	}

	if totalSlots > 0 {
		response.Summary.CompletionRate7Days = float64(completedSlots) / float64(totalSlots) * 100
	}

	return response, nil
}

func normalizeManualHabitInput(req saveManualHabitRequest) (name string, routine string, areaName string, err error) {
	name = strings.TrimSpace(req.Name)
	if name == "" {
		return "", "", "", errors.New("name is required")
	}

	routine = normalizeManualHabitRoutine(req.Routine)
	if routine == "" {
		return "", "", "", errors.New("invalid routine")
	}

	areaName = strings.TrimSpace(req.AreaName)
	return name, routine, areaName, nil
}

func normalizeManualHabitRoutine(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "morning":
		return "morning"
	case "evening":
		return "evening"
	case "anytime", "other", "day":
		return "anytime"
	default:
		return ""
	}
}

func manualHabitRoutine(values []string) string {
	for _, value := range values {
		if normalized := normalizeManualHabitRoutine(value); normalized != "" {
			return normalized
		}
	}
	return "anytime"
}

func normalizeManualHabitStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "completed":
		return "completed"
	case "skipped":
		return "skipped"
	case "failed":
		return "failed"
	case "none", "":
		return "none"
	default:
		return ""
	}
}

func parseManualHabitDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().Truncate(24 * time.Hour)
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		return parsed
	}
	return time.Now().Truncate(24 * time.Hour)
}

func statusForDay(statuses map[string]string, targetDate time.Time) string {
	if statuses == nil {
		return "none"
	}
	if status := normalizeManualHabitStatus(statuses[targetDate.Format("2006-01-02")]); status != "" {
		return status
	}
	return "none"
}

func completedStreak(statuses map[string]string, targetDate time.Time) int {
	streak := 0
	for current := targetDate; !current.Before(targetDate.AddDate(0, 0, -(manualHabitStreakWindowDays - 1))); current = current.AddDate(0, 0, -1) {
		if statusForDay(statuses, current) != "completed" {
			break
		}
		streak++
	}
	return streak
}

func newManualHabitExternalID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "manual-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return "manual-" + hex.EncodeToString(buf)
}
