package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"life-dashboard/internal/connectors"
)

// workoutWriter is the narrow part of the Hevy connector this handler needs. It
// is an interface so the push can be tested without a live account, and so the
// handler stays unaware of api keys and request signing.
type workoutWriter interface {
	CreateWorkout(ctx context.Context, userID string, draft connectors.HevyWorkoutDraft) (string, error)
}

// pushSession writes a finished session to Hevy and records the result.
//
// Only a session that is finished, non-empty and not yet pushed is sent. That is
// what keeps a repeated "закончить тренировку" from creating a second workout:
// the claim below moves the row out of 'finished' before the request goes out, so
// a concurrent attempt finds nothing to claim.
func (h *VoiceWorkoutHandler) pushSession(ctx context.Context, userID, sessionID string, response *voiceWorkoutResponse) {
	if h.hevy == nil {
		return
	}

	draft, title, start, end, ok, err := h.claimSessionForPush(ctx, sessionID)
	if err != nil {
		h.logger.Error().Err(err).Str("session_id", sessionID).Msg("claim session for push")
		return
	}
	if !ok {
		return
	}

	workoutID, err := h.hevy.CreateWorkout(ctx, userID, connectors.HevyWorkoutDraft{
		Title:     title,
		StartTime: start,
		EndTime:   end,
		Exercises: toHevyExercises(draft),
	})
	if err != nil {
		h.logger.Error().Err(err).Str("session_id", sessionID).Msg("push workout to hevy")
		if markErr := h.markPushFailed(ctx, sessionID, err.Error()); markErr != nil {
			h.logger.Warn().Err(markErr).Msg("record push failure")
		}
		response.PushError = err.Error()
		return
	}

	if err := h.markPushed(ctx, sessionID, workoutID); err != nil {
		// The workout exists in Hevy but we failed to record it. Say so loudly:
		// the next attempt would duplicate it.
		h.logger.Error().Err(err).Str("session_id", sessionID).Str("workout_id", workoutID).
			Msg("workout created but not recorded")
	}
	response.HevyWorkoutID = workoutID
}

// claimSessionForPush atomically takes ownership of a finished session, so only
// one attempt can send it.
func (h *VoiceWorkoutHandler) claimSessionForPush(ctx context.Context, sessionID string) ([]voiceParsedExercise, string, time.Time, time.Time, bool, error) {
	var (
		raw             []byte
		title           *string
		started         time.Time
		finished        *time.Time
		durationSeconds *int
	)

	err := h.db.QueryRow(ctx, `
		UPDATE voice_workout_sessions
		SET status = 'pushing', updated_at = NOW()
		WHERE id = $1 AND status = 'finished' AND hevy_workout_id IS NULL
		  AND draft IS NOT NULL AND jsonb_array_length(draft) > 0
		RETURNING draft, title, started_at, finished_at, duration_seconds
	`, sessionID).Scan(&raw, &title, &started, &finished, &durationSeconds)
	if err != nil {
		if isNoRows(err) {
			return nil, "", time.Time{}, time.Time{}, false, nil
		}
		return nil, "", time.Time{}, time.Time{}, false, err
	}

	draft, err := decodeDraft(raw)
	if err != nil {
		return nil, "", time.Time{}, time.Time{}, false, err
	}

	start, end := pushWindow(started, finished, durationSeconds, time.Now())
	return draft, workoutTitle(title, end), start, end, true, nil
}

// pushWindow works out the interval to report to Hevy.
//
// Normally the session brackets itself: the first phrase to the last. A spoken
// duration overrides the start, because in the one-shot mode the whole workout is
// dictated afterwards and the first phrase is not when training began. And the
// interval is forced to be non-empty: a workout dictated in one breath would
// otherwise start and end in the same instant, which Hevy has no reason to
// accept.
func pushWindow(started time.Time, finished *time.Time, durationSeconds *int, now time.Time) (time.Time, time.Time) {
	end := now
	if finished != nil {
		end = *finished
	}

	start := started
	if durationSeconds != nil && *durationSeconds > 0 {
		start = end.Add(-time.Duration(*durationSeconds) * time.Second)
	}
	if !start.Before(end) {
		start = end.Add(-time.Minute)
	}
	return start, end
}

// workoutTitle falls back to a dated title: Hevy requires one, and an empty
// string would land in the history as a nameless workout.
func workoutTitle(title *string, at time.Time) string {
	if title != nil && *title != "" {
		return *title
	}
	return "Тренировка " + at.In(aiDisplayLocation).Format("02.01.2006")
}

func toHevyExercises(draft []voiceParsedExercise) []connectors.HevyExerciseDraft {
	exercises := make([]connectors.HevyExerciseDraft, 0, len(draft))
	for _, exercise := range draft {
		sets := make([]connectors.HevySetDraft, 0, len(exercise.Sets))
		for _, set := range exercise.Sets {
			sets = append(sets, connectors.HevySetDraft{
				Type:            set.Type,
				WeightKg:        set.WeightKg,
				Reps:            set.Reps,
				DurationSeconds: set.DurationSeconds,
			})
		}
		exercises = append(exercises, connectors.HevyExerciseDraft{
			TemplateID: exercise.TemplateID,
			Sets:       sets,
		})
	}
	return exercises
}

func (h *VoiceWorkoutHandler) markPushed(ctx context.Context, sessionID, workoutID string) error {
	_, err := h.db.Exec(ctx, `
		UPDATE voice_workout_sessions
		SET status = 'pushed', hevy_workout_id = $2, push_error = NULL, updated_at = NOW()
		WHERE id = $1
	`, sessionID, workoutID)
	return err
}

// markPushFailed returns the session to 'finished' so a later attempt can retry
// it, and keeps the reason for the reply and the logs.
func (h *VoiceWorkoutHandler) markPushFailed(ctx context.Context, sessionID, reason string) error {
	_, err := h.db.Exec(ctx, `
		UPDATE voice_workout_sessions
		SET status = 'finished', push_error = $2, updated_at = NOW()
		WHERE id = $1
	`, sessionID, reason)
	return err
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func decodeDraft(raw []byte) ([]voiceParsedExercise, error) {
	var draft []voiceParsedExercise
	if err := json.Unmarshal(raw, &draft); err != nil {
		return nil, fmt.Errorf("decode draft: %w", err)
	}
	return draft, nil
}
