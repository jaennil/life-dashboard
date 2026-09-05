package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	authmw "life-dashboard/internal/middleware"
)

// A schedule that came due while the instance was down still fires, but only
// within this window: a report meant for 21:00 is worth reading at 23:00 and
// pointless at nine the next morning.
const checkupScheduleCatchUp = 3 * time.Hour

// CheckupSchedule is one recurring report. Weekday applies to the weekly one and
// DayOfMonth to the monthly one; both are ignored for the daily schedule.
type CheckupSchedule struct {
	Period     string     `json:"period"`
	Enabled    bool       `json:"enabled"`
	Hour       int        `json:"hour"`
	Minute     int        `json:"minute"`
	Weekday    *int       `json:"weekday,omitempty"`
	DayOfMonth *int       `json:"day_of_month,omitempty"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
}

// Defaults chosen to match when the report is worth reading: the day just lived
// through, the week that just ended, and the month after it closed.
func defaultCheckupSchedules() []CheckupSchedule {
	sunday, first := 0, 1
	return []CheckupSchedule{
		{Period: checkupPeriodToday, Hour: 21, Minute: 0},
		{Period: checkupPeriodWeek, Hour: 21, Minute: 0, Weekday: &sunday},
		{Period: checkupPeriodMonth, Hour: 10, Minute: 0, DayOfMonth: &first},
	}
}

func (h *AIHandler) GetCheckupSchedules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)

	stored := map[string]CheckupSchedule{}
	rows, err := h.db.Query(ctx, `
		SELECT period, enabled, hour, minute, weekday, day_of_month, last_run_at
		FROM checkup_schedules
		WHERE user_id = $1
	`, userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("load checkup schedules")
		http.Error(w, "cannot load checkup schedules", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var schedule CheckupSchedule
		if err := rows.Scan(&schedule.Period, &schedule.Enabled, &schedule.Hour, &schedule.Minute,
			&schedule.Weekday, &schedule.DayOfMonth, &schedule.LastRunAt); err != nil {
			h.logger.Error().Err(err).Msg("scan checkup schedule")
			http.Error(w, "cannot load checkup schedules", http.StatusInternalServerError)
			return
		}
		stored[schedule.Period] = schedule
	}

	// Every period is always answered, so the UI draws three rows whether or not
	// the account has ever saved them.
	result := defaultCheckupSchedules()
	for i, fallback := range result {
		if saved, ok := stored[fallback.Period]; ok {
			result[i] = saved
		}
	}
	writeJSONStatus(w, http.StatusOK, result)
}

func (h *AIHandler) SaveCheckupSchedules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(authmw.UserIDKey).(string)

	var schedules []CheckupSchedule
	if err := json.NewDecoder(r.Body).Decode(&schedules); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	for i := range schedules {
		if err := normalizeCheckupSchedule(&schedules[i]); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		h.logger.Error().Err(err).Msg("begin save checkup schedules")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	for _, schedule := range schedules {
		if _, err := tx.Exec(ctx, `
			INSERT INTO checkup_schedules (user_id, period, enabled, hour, minute, weekday, day_of_month, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
			ON CONFLICT (user_id, period) DO UPDATE SET
				enabled = EXCLUDED.enabled,
				hour = EXCLUDED.hour,
				minute = EXCLUDED.minute,
				weekday = EXCLUDED.weekday,
				day_of_month = EXCLUDED.day_of_month,
				updated_at = NOW()
		`, userID, schedule.Period, schedule.Enabled, schedule.Hour, schedule.Minute,
			schedule.Weekday, schedule.DayOfMonth); err != nil {
			h.logger.Error().Err(err).Str("period", schedule.Period).Msg("save checkup schedule")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		h.logger.Error().Err(err).Msg("commit checkup schedules")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.GetCheckupSchedules(w, r)
}

func normalizeCheckupSchedule(schedule *CheckupSchedule) error {
	switch schedule.Period {
	case checkupPeriodToday:
		schedule.Weekday, schedule.DayOfMonth = nil, nil
	case checkupPeriodWeek:
		schedule.DayOfMonth = nil
		if schedule.Weekday == nil {
			sunday := 0
			schedule.Weekday = &sunday
		}
		if *schedule.Weekday < 0 || *schedule.Weekday > 6 {
			return fmt.Errorf("weekday must be between 0 and 6")
		}
	case checkupPeriodMonth:
		schedule.Weekday = nil
		if schedule.DayOfMonth == nil {
			first := 1
			schedule.DayOfMonth = &first
		}
		// 29th to 31st would skip whole months, so the picker stops at 28.
		if *schedule.DayOfMonth < 1 || *schedule.DayOfMonth > 28 {
			return fmt.Errorf("day_of_month must be between 1 and 28")
		}
	default:
		return fmt.Errorf("unsupported checkup period %q", schedule.Period)
	}

	if schedule.Hour < 0 || schedule.Hour > 23 {
		return fmt.Errorf("hour must be between 0 and 23")
	}
	if schedule.Minute < 0 || schedule.Minute > 59 {
		return fmt.Errorf("minute must be between 0 and 59")
	}
	return nil
}

// EnqueueDueCheckups queues the reports whose time has come. It only enqueues:
// the queue worker is what generates them, so a schedule costs the same as
// pressing the button.
func (h *AIHandler) EnqueueDueCheckups(ctx context.Context, now time.Time) int {
	rows, err := h.db.Query(ctx, `
		SELECT user_id, period, hour, minute, weekday, day_of_month, last_run_at
		FROM checkup_schedules
		WHERE enabled = TRUE
	`)
	if err != nil {
		h.logger.Error().Err(err).Msg("load due checkup schedules")
		return 0
	}

	type due struct {
		userID    string
		period    string
		scheduled time.Time
	}
	pending := make([]due, 0, 4)

	for rows.Next() {
		var userID string
		var schedule CheckupSchedule
		if err := rows.Scan(&userID, &schedule.Period, &schedule.Hour, &schedule.Minute,
			&schedule.Weekday, &schedule.DayOfMonth, &schedule.LastRunAt); err != nil {
			h.logger.Error().Err(err).Msg("scan due checkup schedule")
			continue
		}
		if scheduled, ok := checkupScheduleDue(schedule, now); ok {
			pending = append(pending, due{userID: userID, period: schedule.Period, scheduled: scheduled})
		}
	}
	rows.Close()

	queued := 0
	for _, item := range pending {
		jobID, existing, err := h.enqueueCheckupJob(ctx, item.userID, item.period)
		if err != nil {
			h.logger.Error().Err(err).Str("user_id", item.userID).Str("period", item.period).Msg("enqueue scheduled checkup")
			continue
		}

		// Marked as run even when another checkup was already in flight: the
		// schedule fired, and retrying it minutes later would only pile up.
		if _, err := h.db.Exec(ctx, `
			UPDATE checkup_schedules SET last_run_at = $3, updated_at = NOW()
			WHERE user_id = $1 AND period = $2
		`, item.userID, item.period, item.scheduled); err != nil {
			h.logger.Error().Err(err).Str("user_id", item.userID).Msg("record scheduled checkup run")
		}

		h.logger.Info().
			Str("user_id", item.userID).
			Str("period", item.period).
			Str("job_id", jobID).
			Bool("already_running", existing).
			Msg("scheduled checkup queued")
		queued++
	}

	select {
	case h.wake <- struct{}{}:
	default:
	}
	return queued
}

// checkupScheduleDue answers whether this schedule's moment has passed today
// without having been served yet. Times are local to the dashboard's display
// zone, which is what the hour in the settings means to the person who set it.
func checkupScheduleDue(schedule CheckupSchedule, now time.Time) (time.Time, bool) {
	local := now.In(aiDisplayLocation)
	scheduled := time.Date(local.Year(), local.Month(), local.Day(),
		schedule.Hour, schedule.Minute, 0, 0, aiDisplayLocation)

	if local.Before(scheduled) {
		return time.Time{}, false
	}
	if local.Sub(scheduled) > checkupScheduleCatchUp {
		return time.Time{}, false
	}

	switch schedule.Period {
	case checkupPeriodWeek:
		if schedule.Weekday == nil || int(local.Weekday()) != *schedule.Weekday {
			return time.Time{}, false
		}
	case checkupPeriodMonth:
		if schedule.DayOfMonth == nil || local.Day() != *schedule.DayOfMonth {
			return time.Time{}, false
		}
	}

	if schedule.LastRunAt != nil && !schedule.LastRunAt.Before(scheduled) {
		return time.Time{}, false
	}
	return scheduled, true
}
