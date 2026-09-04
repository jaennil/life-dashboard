package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	authmw "life-dashboard/internal/middleware"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	// voiceWorkoutMaxBody is generous for dictated text but still bounded: the
	// body is one spoken phrase, not a file.
	voiceWorkoutMaxBody = 64 << 10
	// voiceWorkoutIdleTimeout closes a session that stopped receiving phrases.
	// Without it a forgotten session would swallow tomorrow's first phrase into
	// yesterday's workout.
	voiceWorkoutIdleTimeout = 4 * time.Hour
)

type VoiceWorkoutHandler struct {
	db *pgxpool.Pool
	// ai is borrowed rather than reconstructed so the provider, model and
	// reasoning effort stay configured in one place. It may be nil, in which case
	// phrases are still archived and simply left unparsed.
	ai *AIHandler
	// Parsing runs on its own model: see the note at the call site.
	parseModel  string
	parseEffort string
	// hevy writes the finished workout. Nil keeps everything else working and
	// simply leaves the draft in the database, which is how this ran while the
	// parse was still being proven.
	hevy workoutWriter
	// food writes dictated meals straight to the FatSecret diary.
	food   foodWriter
	push   *webPushSender
	wake   chan struct{}
	logger zerolog.Logger
}

func NewVoiceWorkout(db *pgxpool.Pool, ai *AIHandler, hevy workoutWriter, food foodWriter, parseModel, parseEffort string, pushOptions WebPushOptions, logger zerolog.Logger) *VoiceWorkoutHandler {
	return &VoiceWorkoutHandler{
		db:          db,
		ai:          ai,
		hevy:        hevy,
		food:        food,
		parseModel:  parseModel,
		parseEffort: parseEffort,
		push:        newWebPushSender(db, pushOptions, logger),
		wake:        make(chan struct{}, 1),
		logger:      logger.With().Str("handler", "voice_workout").Logger(),
	}
}

type voiceWorkoutEnvelope struct {
	APIKey string `json:"api_key"`
	Text   string `json:"text"`
	// Finish ends the session explicitly. It is also inferred from the text, so
	// the Shortcut does not have to send it.
	Finish bool `json:"finish"`
	// DurationMinutes carries the spoken length for the one-shot mode.
	DurationMinutes int `json:"duration_minutes"`
}

type voiceWorkoutResponse struct {
	typed      bool
	Status     string `json:"status"`
	SessionID  string `json:"session_id"`
	Utterances int    `json:"utterances"`
	Finished   bool   `json:"finished"`
	// Heard echoes the recognized text and Understood what was made of this
	// phrase, so the Shortcut can show both while the phone is still in hand.
	// Getting this wrong silently is the main risk of dictating, and this is the
	// only place it can be caught before the workout is written.
	Heard         string   `json:"heard"`
	Understood    string   `json:"understood,omitempty"`
	Workout       string   `json:"workout,omitempty"`
	Unmatched     []string `json:"unmatched,omitempty"`
	Title         string   `json:"title,omitempty"`
	ParseError    string   `json:"parse_error,omitempty"`
	Domain        string   `json:"domain,omitempty"`
	Food          string   `json:"food,omitempty"`
	HevyWorkoutID string   `json:"hevy_workout_id,omitempty"`
	PushError     string   `json:"push_error,omitempty"`
	Message       string   `json:"message,omitempty"`
	// Display is the one field the Shortcut reads: whatever happened, this is
	// what to show.
	Display string `json:"display"`
}

// ReceiveText accepts one dictated phrase. It appends to the open session,
// opening one if needed, and finishes the session when the phrase says so.
//
// POST /api/v1/webhook/voice-workout
func (h *VoiceWorkoutHandler) ReceiveText(w http.ResponseWriter, r *http.Request) {
	_, envelope, ok := readVoiceEnvelope(w, r)
	if !ok {
		return
	}

	apiKey := healthAPIKeyFromRequest(r, envelope.APIKey)
	if apiKey == "" {
		http.Error(w, "api_key required", http.StatusUnauthorized)
		return
	}

	var userID string
	if err := h.db.QueryRow(r.Context(),
		`SELECT user_id FROM api_keys WHERE key = $1`, apiKey).Scan(&userID); err != nil {
		http.Error(w, "invalid api_key", http.StatusUnauthorized)
		return
	}

	h.enqueueText(w, r, userID, envelope, false)
}

// ReceiveTypedText accepts text from the authenticated web application. It
// deliberately shares all routing and write behavior with dictated input while
// using the signed-in user instead of exposing a webhook API key to the browser.
func (h *VoiceWorkoutHandler) ReceiveTypedText(w http.ResponseWriter, r *http.Request) {
	_, envelope, ok := readVoiceEnvelope(w, r)
	if !ok {
		return
	}

	userID, ok := r.Context().Value(authmw.UserIDKey).(string)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	h.enqueueText(w, r, userID, envelope, true)
}

func readVoiceEnvelope(w http.ResponseWriter, r *http.Request) ([]byte, voiceWorkoutEnvelope, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, voiceWorkoutMaxBody))
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return nil, voiceWorkoutEnvelope{}, false
	}

	var envelope voiceWorkoutEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return nil, voiceWorkoutEnvelope{}, false
	}
	return body, envelope, true
}

func (h *VoiceWorkoutHandler) processText(ctx context.Context, userID, eventID string, envelope voiceWorkoutEnvelope, typed bool) (voiceWorkoutResponse, error) {
	text := normalizeVoiceText(envelope.Text)
	if !voiceHasContent(text) {
		// Starting dictation and saying nothing is a normal thing to do at a
		// machine, and iOS hands it over as "" or as a lone ".". It used to come
		// back as a 400 with a plain-text body, which fails the Shortcut outright
		// and shows a system error dialog instead of our text. Nothing is archived
		// and no model is called - there is nothing to interpret - but the phone
		// gets a proper answer.
		display := "Ничего не услышал. Повтори."
		if typed {
			display = "Нечего отправлять. Введи текст."
		}
		return voiceWorkoutResponse{
			typed:   typed,
			Status:  "ok",
			Domain:  voiceDomainUnknown,
			Heard:   text,
			Display: display,
		}, nil
	}

	finish := envelope.Finish || looksLikeWorkoutFinish(text)
	now := time.Now()

	openSessionID, workoutOpen, err := h.findOpenSession(ctx, userID, now)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("find open session")
		return voiceWorkoutResponse{}, err
	}

	response := voiceWorkoutResponse{typed: typed, Status: "ok", Heard: text}

	spoken := stripFinishPhrase(text)
	interpreted := voiceInterpretation{Domain: voiceDomainWorkout}
	if spoken != "" {
		// A phrase that is nothing but the finish command needs no upstream call,
		// and while a workout is open the finish itself is unambiguous.
		interpreted = h.classify(ctx, userID, spoken, openSessionID, workoutOpen, &response)
	}
	response.Domain = interpreted.Domain
	// A parser failure is retried by the durable worker. Do not mutate a workout
	// session on the fallback domain or the retry would append the phrase twice.
	if response.ParseError != "" {
		response.Display = composeVoiceDisplay(response)
		return response, errors.New(response.ParseError)
	}
	if err := h.recordPhraseDomain(ctx, eventID, interpreted.Domain); err != nil {
		h.logger.Warn().Err(err).Str("event_id", eventID).Msg("record phrase domain")
	}

	if interpreted.Domain != voiceDomainWorkout {
		// Not a workout: no session is opened and none is touched. The reply says
		// where the phrase was routed so a misclassification is visible at once.
		switch {
		case interpreted.Domain == voiceDomainFood:
			h.applyFood(ctx, userID, eventID, interpreted, &response)
		default:
			if reply, known := voiceDomainReplies[interpreted.Domain]; known {
				response.Message = reply
			} else {
				response.Message = "Не понял, к чему отнести фразу. Она сохранена."
			}
		}
		response.Display = composeVoiceDisplay(response)
		h.logger.Info().Str("user_id", userID).Str("domain", interpreted.Domain).Msg("voice phrase routed")
		return response, nil
	}

	sessionID := openSessionID
	if !workoutOpen {
		if sessionID, err = h.createSession(ctx, userID, now); err != nil {
			h.logger.Error().Err(err).Str("user_id", userID).Msg("create session")
			return response, err
		}
	} else if err := h.touchSession(ctx, sessionID, now); err != nil {
		h.logger.Warn().Err(err).Str("session_id", sessionID).Msg("touch session")
	}
	response.SessionID = sessionID
	response.Finished = finish

	if err := h.appendUtterance(ctx, sessionID, text, finish, now); err != nil {
		h.logger.Error().Err(err).Str("session_id", sessionID).Msg("append utterance")
		return response, err
	}

	if interpreted.Parsed != nil {
		h.applyParsed(ctx, sessionID, interpreted, &response)
	}

	if envelope.DurationMinutes > 0 {
		if err := h.setSpokenDuration(ctx, sessionID, envelope.DurationMinutes); err != nil {
			h.logger.Warn().Err(err).Str("session_id", sessionID).Msg("store spoken duration")
		}
	}

	if finish {
		title := h.generateTitle(ctx, sessionID)
		if err := h.finishSession(ctx, sessionID, now, title); err != nil {
			h.logger.Error().Err(err).Str("session_id", sessionID).Msg("finish session")
			return response, err
		}
		response.Title = title
		h.pushSession(ctx, userID, sessionID, &response)
	}

	count, err := h.countUtterances(ctx, sessionID)
	if err != nil {
		h.logger.Warn().Err(err).Str("session_id", sessionID).Msg("count utterances")
	}
	response.Utterances = count
	response.Display = composeVoiceDisplay(response)

	h.logger.Info().Str("user_id", userID).Str("session_id", sessionID).
		Bool("finished", finish).Int("utterances", count).
		Int("unmatched", len(response.Unmatched)).Msg("voice phrase stored")

	return response, nil
}

// findOpenSession returns the user's open workout session if there is a live one,
// closing a stale one on the way. It deliberately does not create anything: the
// domain of the phrase is not known yet, and a phrase about food must not open a
// workout.
func (h *VoiceWorkoutHandler) findOpenSession(ctx context.Context, userID string, now time.Time) (string, bool, error) {
	var sessionID string
	var lastUtteranceAt time.Time

	err := h.db.QueryRow(ctx, `
		SELECT id, last_utterance_at FROM voice_workout_sessions
		WHERE user_id = $1 AND status = 'open'
	`, userID).Scan(&sessionID, &lastUtteranceAt)

	switch {
	case err == nil && now.Sub(lastUtteranceAt) <= voiceWorkoutIdleTimeout:
		return sessionID, true, nil
	case err == nil:
		// A stale session is closed without a title: it was abandoned rather
		// than finished, and naming it would imply otherwise.
		if err := h.finishSession(ctx, sessionID, lastUtteranceAt, ""); err != nil {
			return "", false, err
		}
		return "", false, nil
	case errors.Is(err, pgx.ErrNoRows):
		return "", false, nil
	default:
		return "", false, err
	}
}

// touchSession records that a phrase arrived, which is what the idle close
// measures against.
func (h *VoiceWorkoutHandler) touchSession(ctx context.Context, sessionID string, now time.Time) error {
	_, err := h.db.Exec(ctx, `
		UPDATE voice_workout_sessions SET last_utterance_at = $2, updated_at = NOW()
		WHERE id = $1
	`, sessionID, now)
	return err
}

func (h *VoiceWorkoutHandler) createSession(ctx context.Context, userID string, now time.Time) (string, error) {
	var sessionID string
	err := h.db.QueryRow(ctx, `
		INSERT INTO voice_workout_sessions (user_id, started_at, last_utterance_at)
		VALUES ($1, $2, $2)
		RETURNING id
	`, userID, now).Scan(&sessionID)
	return sessionID, err
}

func (h *VoiceWorkoutHandler) appendUtterance(ctx context.Context, sessionID, text string, isFinish bool, now time.Time) error {
	_, err := h.db.Exec(ctx, `
		INSERT INTO voice_workout_utterances (session_id, said_at, text, is_finish)
		VALUES ($1, $2, $3, $4)
	`, sessionID, now, text, isFinish)
	return err
}

func (h *VoiceWorkoutHandler) setSpokenDuration(ctx context.Context, sessionID string, minutes int) error {
	_, err := h.db.Exec(ctx, `
		UPDATE voice_workout_sessions SET duration_seconds = $2, updated_at = NOW()
		WHERE id = $1
	`, sessionID, minutes*60)
	return err
}

func (h *VoiceWorkoutHandler) finishSession(ctx context.Context, sessionID string, at time.Time, title string) error {
	_, err := h.db.Exec(ctx, `
		UPDATE voice_workout_sessions
		SET status = 'finished', finished_at = $2, title = COALESCE(NULLIF($3, ''), title),
		    updated_at = NOW()
		WHERE id = $1 AND status = 'open'
	`, sessionID, at, title)
	return err
}

func (h *VoiceWorkoutHandler) countUtterances(ctx context.Context, sessionID string) (int, error) {
	var count int
	err := h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM voice_workout_utterances WHERE session_id = $1`, sessionID).Scan(&count)
	return count, err
}

// CloseIdleSessions finishes sessions that stopped receiving phrases. The lazy
// close in openOrResumeSession only fires when the next phrase arrives, which
// may be days later or never, so the sweep is what actually bounds a session.
func (h *VoiceWorkoutHandler) CloseIdleSessions(ctx context.Context) (int, error) {
	rows, err := h.db.Exec(ctx, `
		UPDATE voice_workout_sessions
		SET status = 'finished', finished_at = last_utterance_at, updated_at = NOW()
		WHERE status = 'open' AND last_utterance_at < NOW() - $1::interval
	`, voiceWorkoutIdleTimeout.String())
	if err != nil {
		return 0, err
	}
	return int(rows.RowsAffected()), nil
}

// normalizeVoiceText tidies dictated text. It leans on the Screen Time
// normalizer because dictation arrives through the same iOS pipeline and carries
// the same exotic spaces and invisible marks, which would otherwise defeat the
// finish-phrase match. Whitespace is collapsed so "закончить  тренировку" and a
// line break in the middle of the phrase both still match.
func normalizeVoiceText(text string) string {
	return strings.Join(strings.Fields(normalizeScreenTimeText(text)), " ")
}

// voiceFinishPhrases are the spoken ways of ending a workout. Matching is on a
// normalized lowercase form and tolerates the trailing punctuation dictation
// likes to add.
var voiceFinishPhrases = []string{
	"закончить тренировку",
	"закончил тренировку",
	"конец тренировки",
	"тренировка закончена",
	"завершить тренировку",
	"finish workout",
	"end workout",
}

func looksLikeWorkoutFinish(text string) bool {
	lowered := strings.ToLower(strings.Trim(normalizeVoiceText(text), ".!?,"))
	for _, phrase := range voiceFinishPhrases {
		if strings.Contains(lowered, phrase) {
			return true
		}
	}
	return false
}

// voiceHasContent reports whether a phrase carries anything to interpret. A lone
// "." or "..." is what dictation produces for silence; "Ммм." is a phrase.
func voiceHasContent(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
