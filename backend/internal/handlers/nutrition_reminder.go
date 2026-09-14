package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"life-dashboard/internal/connectors"
)

// MealReminderHandler nudges the user when a meal they normally log is still
// missing an hour after they would normally have eaten it.
type MealReminderHandler struct {
	db       *pgxpool.Pool
	push     *webPushSender
	telegram *TelegramHandler
	syncer   aiConnectorSyncer
	logger   zerolog.Logger
}

func NewMealReminder(db *pgxpool.Pool, pushOptions WebPushOptions, telegram *TelegramHandler, logger zerolog.Logger) *MealReminderHandler {
	return &MealReminderHandler{
		db:       db,
		push:     newWebPushSender(db, pushOptions, logger),
		telegram: telegram,
		logger:   logger.With().Str("handler", "meal_reminder").Logger(),
	}
}

// UseSyncer lets the reminder refresh the diary before it decides anything. It
// is optional: without it the check runs on whatever the last scheduled sync
// brought in.
func (h *MealReminderHandler) UseSyncer(syncer aiConnectorSyncer) { h.syncer = syncer }

// mealDeadlines is when each meal stops being plausible and starts being
// forgotten, in local time.
//
// The hours are the meal windows the rest of the app already uses - breakfast
// until 11, lunch until 16, dinner until 21 - plus the hour the person asked
// for. Snacks are absent on purpose: there is no time by which a snack is late.
var mealDeadlines = map[string]int{
	mealBreakfast: 12,
	mealLunch:     17,
	mealDinner:    22,
}

// mealReminderWindow is how long a missed meal stays worth mentioning.
//
// Past it the nudge stops being a reminder and becomes a reproach about
// something already over: nobody logs the breakfast they were reminded of at
// four in the afternoon. A meal missed by more than this is left to the checkup.
const mealReminderWindow = 3 * time.Hour

// mealReminderHistoryDays is how far back the habit is measured.
const mealReminderHistoryDays = 30

// mealReminderMinTrackedDays keeps a new or barely used diary quiet. Two days of
// history cannot tell a habit from a coincidence, and a reminder built on a
// coincidence is just noise.
const mealReminderMinTrackedDays = 5

// mealHabitShare is how often a meal has to appear among the tracked days before
// its absence means anything.
const mealHabitShare = 0.5

// mealReminderCandidate is one meal that might need a nudge.
type mealReminderCandidate struct {
	Meal     string
	Deadline int
}

// dueMeals picks the meals whose deadline has passed, latest first.
//
// The latest one wins because it is the one the person can still act on: at nine
// in the evening, "ужин не записан" is useful and "завтрак не записан" is a
// reproach about something twelve hours gone.
func dueMeals(now time.Time) []mealReminderCandidate {
	due := make([]mealReminderCandidate, 0, len(mealDeadlines))
	for meal, hour := range mealDeadlines {
		deadline := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
		if now.Before(deadline) || now.Sub(deadline) >= mealReminderWindow {
			continue
		}
		due = append(due, mealReminderCandidate{Meal: meal, Deadline: hour})
	}
	for i := 0; i < len(due); i++ {
		for j := i + 1; j < len(due); j++ {
			if due[j].Deadline > due[i].Deadline {
				due[i], due[j] = due[j], due[i]
			}
		}
	}
	return due
}

// mealIsHabitual reports whether the absence of this meal says anything.
func mealIsHabitual(daysWithMeal, trackedDays int) bool {
	if trackedDays < mealReminderMinTrackedDays {
		return false
	}
	return float64(daysWithMeal) >= float64(trackedDays)*mealHabitShare
}

// Run checks every user once. It is called from the scheduler and is safe to
// call more often than the deadlines: a meal is reminded about once a day.
func (h *MealReminderHandler) Run(ctx context.Context) {
	now := time.Now().In(aiDisplayLocation)
	due := dueMeals(now)
	if len(due) == 0 {
		return
	}

	rows, err := h.db.Query(ctx, `
		SELECT user_id FROM sync_state WHERE source = 'fatsecret' AND enabled = TRUE
	`)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load users for meal reminders")
		return
	}
	users := make([]string, 0, 4)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			rows.Close()
			h.logger.Warn().Err(err).Msg("scan user for meal reminders")
			return
		}
		users = append(users, userID)
	}
	rows.Close()

	for _, userID := range users {
		if err := h.remindUser(ctx, userID, now, due); err != nil {
			h.logger.Warn().Err(err).Str("user_id", userID).Msg("meal reminder")
		}
	}
}

func (h *MealReminderHandler) remindUser(ctx context.Context, userID string, now time.Time, due []mealReminderCandidate) error {
	today := now.Format("2006-01-02")

	// One unanswered nudge a day is enough. If the last one changed nothing, a
	// second is nagging, and the person has already been told.
	answered, err := h.remindedTodayUnanswered(ctx, userID, today)
	if err != nil {
		return err
	}
	if answered {
		return nil
	}

	// The diary is read from FatSecret on a timer, so a meal logged in the app
	// minutes ago may not be here yet. Refreshing first is what keeps the
	// reminder from being about our own lag.
	h.refreshDiary(ctx, userID)

	logged, err := h.loggedMealsToday(ctx, userID, today)
	if err != nil {
		return err
	}
	habits, trackedDays, err := h.mealHabits(ctx, userID)
	if err != nil {
		return err
	}

	for _, candidate := range due {
		if logged[candidate.Meal] {
			continue
		}
		if !mealIsHabitual(habits[candidate.Meal], trackedDays) {
			continue
		}
		sent, err := h.claimReminder(ctx, userID, today, candidate.Meal)
		if err != nil {
			return err
		}
		if !sent {
			continue
		}
		h.deliver(ctx, userID, candidate.Meal)
		// One meal per pass: the loop is ordered latest-first, so this is the one
		// that is still worth saying.
		return nil
	}
	return nil
}

// remindedTodayUnanswered reports whether today's reminder was already sent and
// nothing has been logged since.
func (h *MealReminderHandler) remindedTodayUnanswered(ctx context.Context, userID, today string) (bool, error) {
	var pending bool
	err := h.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM meal_reminders r
			WHERE r.user_id = $1 AND r.date = $2::date
			  AND NOT EXISTS (
				SELECT 1 FROM nutrition_daily d
				JOIN nutrition_items i ON i.daily_id = d.id
				WHERE d.user_id = $1 AND d.date = $2::date AND i.meal_type = r.meal
			  )
		)
	`, userID, today).Scan(&pending)
	if err != nil {
		return false, fmt.Errorf("check today's reminders: %w", err)
	}
	return pending, nil
}

func (h *MealReminderHandler) refreshDiary(ctx context.Context, userID string) {
	if h.syncer == nil {
		return
	}
	syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := h.syncer.SyncNow(syncCtx, "fatsecret", userID, connectors.SyncTriggerPrefetch); err != nil {
		h.logger.Warn().Err(err).Str("user_id", userID).Msg("refresh diary before reminding")
	}
}

func (h *MealReminderHandler) loggedMealsToday(ctx context.Context, userID, today string) (map[string]bool, error) {
	rows, err := h.db.Query(ctx, `
		SELECT DISTINCT i.meal_type
		FROM nutrition_daily d
		JOIN nutrition_items i ON i.daily_id = d.id
		WHERE d.user_id = $1 AND d.date = $2::date
	`, userID, today)
	if err != nil {
		return nil, fmt.Errorf("load today's meals: %w", err)
	}
	defer rows.Close()

	logged := make(map[string]bool, 4)
	for rows.Next() {
		var meal string
		if err := rows.Scan(&meal); err != nil {
			return nil, err
		}
		logged[meal] = true
	}
	return logged, rows.Err()
}

// mealHabits counts the days each meal was logged, and the days anything at all
// was logged, which is what turns a count into a habit.
func (h *MealReminderHandler) mealHabits(ctx context.Context, userID string) (map[string]int, int, error) {
	rows, err := h.db.Query(ctx, `
		SELECT i.meal_type, COUNT(DISTINCT d.date)
		FROM nutrition_daily d
		JOIN nutrition_items i ON i.daily_id = d.id
		WHERE d.user_id = $1 AND d.date > CURRENT_DATE - $2::int AND d.date < CURRENT_DATE
		GROUP BY i.meal_type
	`, userID, mealReminderHistoryDays)
	if err != nil {
		return nil, 0, fmt.Errorf("load meal habits: %w", err)
	}
	defer rows.Close()

	habits := make(map[string]int, 4)
	for rows.Next() {
		var meal string
		var days int
		if err := rows.Scan(&meal, &days); err != nil {
			return nil, 0, err
		}
		habits[meal] = days
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var trackedDays int
	if err := h.db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT d.date)
		FROM nutrition_daily d
		JOIN nutrition_items i ON i.daily_id = d.id
		WHERE d.user_id = $1 AND d.date > CURRENT_DATE - $2::int AND d.date < CURRENT_DATE
	`, userID, mealReminderHistoryDays).Scan(&trackedDays); err != nil {
		return nil, 0, fmt.Errorf("count tracked days: %w", err)
	}
	return habits, trackedDays, nil
}

// claimReminder writes the reminder down before it is sent, and reports whether
// this pass is the one that gets to send it.
func (h *MealReminderHandler) claimReminder(ctx context.Context, userID, today, meal string) (bool, error) {
	tag, err := h.db.Exec(ctx, `
		INSERT INTO meal_reminders (user_id, date, meal) VALUES ($1, $2::date, $3)
		ON CONFLICT (user_id, date, meal) DO NOTHING
	`, userID, today, meal)
	if err != nil {
		return false, fmt.Errorf("claim reminder: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (h *MealReminderHandler) deliver(ctx context.Context, userID, meal string) {
	title := mealReminderTitle(meal)
	body := "Если ел - продиктуй, запишу."

	if err := h.push.send(ctx, userID, "meal-"+meal, map[string]string{
		"title": title, "body": body, "url": "/nutrition",
		// One tag per meal per day: a repeat replaces the notification instead of
		// stacking a second copy on the lock screen.
		"tag": "meal-reminder-" + meal,
	}); err != nil {
		h.logger.Warn().Err(err).Str("user_id", userID).Msg("send meal reminder push")
	}

	if h.telegram != nil {
		if _, err := h.telegram.SendReport(ctx, userID, title, body); err != nil {
			h.logger.Warn().Err(err).Str("user_id", userID).Msg("send meal reminder to telegram")
		}
	}

	h.logger.Info().Str("user_id", userID).Str("meal", meal).Msg("meal reminder sent")
}

func mealReminderTitle(meal string) string {
	return mealLabel(meal) + " не записан"
}
