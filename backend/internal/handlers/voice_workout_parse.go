package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// voiceCandidateLimit bounds how many exercises are offered to the model. The
// full Hevy catalogue is around 460 entries, but the account actually uses a few
// dozen, and a short focused list matches far better than a long one.
const voiceCandidateLimit = 80

// Set bounds. They exist to catch speech recognition, not to judge training:
// "13.5" heard as "135" or a rep count heard as a weight should be rejected
// rather than written into the history.
const (
	voiceMaxReps     = 200
	voiceMaxWeightKg = 500.0
)

// voiceExerciseCandidate is one exercise the model may choose from.
type voiceExerciseCandidate struct {
	TemplateID string `json:"template_id"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Equipment  string `json:"equipment,omitempty"`
	// Times is how often the account has logged this exercise. It is sent to the
	// model because frequency is the best tie-breaker available: between "Pull
	// Up" logged eleven times and a near-identical variant never used, the
	// former is almost always what was meant.
	Times int `json:"times_logged"`
}

// voiceParsedSet mirrors one Hevy set. Pointers distinguish "not spoken" from
// zero, which matters because a reps_only exercise legitimately has no weight.
type voiceParsedSet struct {
	Type            string   `json:"type"`
	Reps            *int     `json:"reps,omitempty"`
	WeightKg        *float64 `json:"weight_kg,omitempty"`
	DurationSeconds *int     `json:"duration_seconds,omitempty"`
}

type voiceParsedExercise struct {
	TemplateID string           `json:"template_id"`
	Title      string           `json:"title"`
	Sets       []voiceParsedSet `json:"sets"`
}

// voiceParseResult is what the model must return. Domain and the workout parse
// come back from the same call: classifying first and parsing second would double
// the wait of someone standing at a machine. Unmatched carries what could not be
// resolved, which is reported back rather than guessed at.
type voiceParseResult struct {
	Domain    string                `json:"domain"`
	Exercises []voiceParsedExercise `json:"exercises"`
	Unmatched []string              `json:"unmatched"`
}

// voiceInterpretation is the router's verdict on a phrase. Parsed stays nil when
// nothing usable came back, which is what tells the caller not to touch the draft.
type voiceInterpretation struct {
	Domain    string
	Parsed    []voiceParsedExercise
	Unmatched []string
}

// loadCandidates builds the exercise shortlist: everything the account has
// actually logged, plus its custom exercises even if never used yet.
//
// The template id is recovered from the archived Hevy payloads because
// workout_sets keeps only the exercise name, and writing a workout back needs
// the id.
func (h *VoiceWorkoutHandler) loadCandidates(ctx context.Context, userID string) ([]voiceExerciseCandidate, error) {
	rows, err := h.db.Query(ctx, `
		WITH logged AS (
			SELECT e->>'title' AS title,
			       e->>'exercise_template_id' AS template_id,
			       COUNT(*) AS times
			FROM raw_events,
			     LATERAL jsonb_array_elements(payload->'exercises') AS e
			WHERE source = 'hevy' AND event_type = 'workout' AND user_id = $1
			  AND e->>'exercise_template_id' IS NOT NULL
			GROUP BY 1, 2
		),
		candidates AS (
			SELECT COALESCE(t.id, logged.template_id) AS template_id,
			       COALESCE(t.title, logged.title)    AS title,
			       COALESCE(t.type, 'weight_reps')    AS type,
			       t.equipment                        AS equipment,
			       logged.times                       AS times
			FROM logged
			LEFT JOIN hevy_exercise_templates t ON t.id = logged.template_id
			UNION
			SELECT t.id, t.title, t.type, t.equipment, 0
			FROM hevy_exercise_templates t
			WHERE t.is_custom AND t.owner_user_id = $1
		)
		SELECT template_id, title, type, COALESCE(equipment, ''), MAX(times)
		FROM candidates
		GROUP BY template_id, title, type, equipment
		ORDER BY MAX(times) DESC, title
		LIMIT $2
	`, userID, voiceCandidateLimit)
	if err != nil {
		return nil, fmt.Errorf("load exercise candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]voiceExerciseCandidate, 0, voiceCandidateLimit)
	for rows.Next() {
		var c voiceExerciseCandidate
		if err := rows.Scan(&c.TemplateID, &c.Title, &c.Type, &c.Equipment, &c.Times); err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// buildVoiceParsePrompt states the rules the dictated phrasing actually needs.
// Every rule here comes from a real phrasing rather than from imagination, and
// the hard constraint is that a template_id must be copied from the list: an
// invented id would be rejected by Hevy, or worse, accepted as another exercise.
func buildVoiceParsePrompt(candidates []voiceExerciseCandidate, draft []voiceParsedExercise, workoutOpen bool) string {
	var sb strings.Builder
	sb.WriteString("Ты разбираешь фразу, надиктованную вслух по-русски в дневник пользователя.\n")
	sb.WriteString("Верни только JSON, без markdown и без пояснений, в формате:\n")
	sb.WriteString(`{"domain":"workout","exercises":[{"template_id":"...","title":"...","sets":[{"type":"normal","reps":5,"weight_kg":13.5}]}],"unmatched":["..."]}`)
	sb.WriteString("\n\nСначала определи domain - о чём фраза:\n")
	sb.WriteString("- workout: упражнения, подходы, повторения, веса.\n")
	sb.WriteString("- food: съеденное, продукты, граммы, калории.\n")
	sb.WriteString("- weight: собственный вес пользователя.\n")
	sb.WriteString("- note: всё остальное, мысли и заметки.\n")
	sb.WriteString("Если domain не workout, верни только domain, а exercises оставь пустым массивом.\n")
	if workoutOpen {
		sb.WriteString("Сейчас у пользователя открыта тренировка. Короткая фраза с числами почти наверняка workout: \"ещё 8\" или \"12 по 30\" - это подходы.\n")
	}
	sb.WriteString("\nПравила разбора тренировки:\n")
	sb.WriteString("- template_id обязан быть скопирован из списка упражнений ниже. Не придумывай новые id.\n")
	sb.WriteString("- Если упражнение не удалось сопоставить со списком, не угадывай: положи исходную формулировку в unmatched.\n")
	sb.WriteString("- В unmatched попадают только неопознанные упражнения. Уточнения техники - хват, наклон, темп - не упражнения: учти их при выборе варианта и не выноси в unmatched.\n")
	sb.WriteString("- \"5 подтягиваний, на втором подходе 4 раза\" - это два подхода: 5 и 4.\n")
	sb.WriteString("- Если количество подходов не названо, это один подход.\n")
	sb.WriteString("- \"3 подхода по 12\" - три одинаковых подхода по 12 повторений.\n")
	sb.WriteString("- \"11 раз, 2 подхода\" - тоже два одинаковых подхода по 11. Если число подходов названо, а повторения названы один раз, повтори это число в каждом подходе. Никогда не ставь reps = 0.\n")
	sb.WriteString("- \"с двумя гантелями по 13.5 кг каждая\" - вес одной гантели, то есть weight_kg = 13.5, а не 27.\n")
	sb.WriteString("- Слово \"поход\" в речи означает \"подход\".\n")
	sb.WriteString("- type подхода: normal, если не сказано иное; warmup для разминочных, failure до отказа, dropset для дропсета.\n")
	sb.WriteString("- Поля подхода зависят от типа упражнения из списка:\n")
	sb.WriteString("  weight_reps и bodyweight_weighted - reps и weight_kg;\n")
	sb.WriteString("  reps_only и bodyweight_assisted - только reps, weight_kg не указывай;\n")
	sb.WriteString("  duration - duration_seconds вместо reps.\n")
	sb.WriteString("- Не додумывай веса и повторения, которых не было в речи.\n")

	if len(draft) > 0 {
		sb.WriteString("\nВ этой тренировке уже записано (не дублируй, только добавляй новое из фразы):\n")
		for _, exercise := range draft {
			sb.WriteString(fmt.Sprintf("- %s: %d подходов\n", exercise.Title, len(exercise.Sets)))
		}
	}

	sb.WriteString("\nДоступные упражнения (times_logged - как часто пользователь их делает, это лучший критерий при неоднозначности):\n")
	encoded, err := json.Marshal(candidates)
	if err == nil {
		sb.Write(encoded)
	}
	sb.WriteString("\n")
	return sb.String()
}

// parsePhrase asks the model to turn one phrase into exercises and sets.
func (h *VoiceWorkoutHandler) parsePhrase(ctx context.Context, userID, text string, draft []voiceParsedExercise, workoutOpen bool) (voiceParseResult, []voiceExerciseCandidate, error) {
	var result voiceParseResult

	candidates, err := h.loadCandidates(ctx, userID)
	if err != nil {
		return result, nil, err
	}
	if len(candidates) == 0 {
		return result, nil, fmt.Errorf("no exercise candidates: sync Hevy first")
	}

	messages := []ChatMessage{
		{Role: "system", Content: buildVoiceParsePrompt(candidates, draft, workoutOpen)},
		{Role: "user", Content: text},
	}

	// Parsing is extraction, not reasoning. On the default checkup model the same
	// phrase took 43 seconds and 2 rubles against 5 seconds and 9 kopecks on the
	// fast one, and 43 seconds standing at a machine is not usable.
	answer, err := h.ai.CompleteWithModel(ctx, "voice_workout_parse", messages, h.parseModel, h.parseEffort)
	if err != nil {
		return result, candidates, err
	}

	parsed, err := decodeVoiceParseResult(answer)
	if err != nil {
		return result, candidates, err
	}
	return parsed, candidates, nil
}

// decodeVoiceParseResult unwraps the model's answer. Models fence JSON in
// markdown even when told not to, so the fence is stripped rather than treated
// as a failure.
func decodeVoiceParseResult(answer string) (voiceParseResult, error) {
	var result voiceParseResult

	cleaned := strings.TrimSpace(answer)
	if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		if end := strings.LastIndex(cleaned, "```"); end >= 0 {
			cleaned = cleaned[:end]
		}
		cleaned = strings.TrimSpace(cleaned)
	}
	if start := strings.Index(cleaned, "{"); start > 0 {
		cleaned = cleaned[start:]
	}

	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return result, fmt.Errorf("decode parse result: %w", err)
	}
	return result, nil
}

// stripFinishPhrase removes the finish command from a phrase and returns what is
// left. "5 подтягиваний, закончить тренировку" carries both an exercise and the
// end of the session, so the exercise must survive the finish.
func stripFinishPhrase(text string) string {
	remainder := text
	lowered := strings.ToLower(remainder)
	for _, phrase := range voiceFinishPhrases {
		for {
			at := strings.Index(lowered, phrase)
			if at < 0 {
				break
			}
			remainder = remainder[:at] + remainder[at+len(phrase):]
			lowered = strings.ToLower(remainder)
		}
	}
	return strings.Trim(strings.Join(strings.Fields(remainder), " "), " ,.;:!?-и")
}

// classify runs the single upstream call that both routes the phrase and parses
// it when it is about training. Failures are recorded and reported but never
// fatal: the phrase is already archived, so a bad parse costs an interpretation,
// not the data.
func (h *VoiceWorkoutHandler) classify(ctx context.Context, userID, text, sessionID string, workoutOpen bool, response *voiceWorkoutResponse) voiceInterpretation {
	if h.ai == nil {
		response.ParseError = "ai upstream is not configured"
		return voiceInterpretation{Domain: resolveVoiceDomain("", workoutOpen)}
	}

	var draft []voiceParsedExercise
	if sessionID != "" {
		loaded, err := h.loadDraft(ctx, sessionID)
		if err != nil {
			h.logger.Warn().Err(err).Str("session_id", sessionID).Msg("load draft")
		} else {
			draft = loaded
		}
	}

	parsed, candidates, err := h.parsePhrase(ctx, userID, text, draft, workoutOpen)
	if err != nil {
		h.logger.Warn().Err(err).Msg("parse phrase")
		response.ParseError = err.Error()
		return voiceInterpretation{Domain: resolveVoiceDomain("", workoutOpen)}
	}

	domain := resolveVoiceDomain(parsed.Domain, workoutOpen)
	if domain != voiceDomainWorkout {
		return voiceInterpretation{Domain: domain}
	}

	kept, rejected := validateParsedExercises(parsed.Exercises, candidates)
	return voiceInterpretation{
		Domain:    domain,
		Parsed:    kept,
		Unmatched: append(append([]string{}, parsed.Unmatched...), rejected...),
	}
}

// applyParsed folds an interpretation into the session draft and fills in what
// the phone will show.
func (h *VoiceWorkoutHandler) applyParsed(ctx context.Context, sessionID string, interpreted voiceInterpretation, response *voiceWorkoutResponse) {
	if err := h.storeUtteranceParse(ctx, sessionID, interpreted.Parsed, strings.Join(interpreted.Unmatched, "; ")); err != nil {
		h.logger.Warn().Err(err).Str("session_id", sessionID).Msg("store utterance parse")
	}

	draft, err := h.loadDraft(ctx, sessionID)
	if err != nil {
		h.logger.Warn().Err(err).Str("session_id", sessionID).Msg("reload draft")
	}

	merged := mergeVoiceDraft(draft, interpreted.Parsed)
	if err := h.saveDraft(ctx, sessionID, merged); err != nil {
		h.logger.Warn().Err(err).Str("session_id", sessionID).Msg("save draft")
	}

	response.Understood = summarizeVoiceDraft(interpreted.Parsed)
	response.Workout = summarizeVoiceDraft(merged)
	response.Unmatched = interpreted.Unmatched
}

func (h *VoiceWorkoutHandler) loadDraft(ctx context.Context, sessionID string) ([]voiceParsedExercise, error) {
	var raw []byte
	if err := h.db.QueryRow(ctx,
		`SELECT COALESCE(draft, '[]'::jsonb) FROM voice_workout_sessions WHERE id = $1`,
		sessionID).Scan(&raw); err != nil {
		return nil, err
	}

	var draft []voiceParsedExercise
	if err := json.Unmarshal(raw, &draft); err != nil {
		return nil, fmt.Errorf("decode draft: %w", err)
	}
	return draft, nil
}

func (h *VoiceWorkoutHandler) saveDraft(ctx context.Context, sessionID string, draft []voiceParsedExercise) error {
	encoded, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	_, err = h.db.Exec(ctx, `
		UPDATE voice_workout_sessions SET draft = $2::jsonb, updated_at = NOW() WHERE id = $1
	`, sessionID, encoded)
	return err
}

// storeUtteranceParse attaches the interpretation to the most recent phrase of
// the session, which is the one just inserted.
func (h *VoiceWorkoutHandler) storeUtteranceParse(ctx context.Context, sessionID string, exercises []voiceParsedExercise, parseError string) error {
	var encoded []byte
	if exercises != nil {
		var err error
		if encoded, err = json.Marshal(exercises); err != nil {
			return err
		}
	}

	_, err := h.db.Exec(ctx, `
		UPDATE voice_workout_utterances
		SET parsed = $2::jsonb, parse_error = NULLIF($3, '')
		WHERE id = (
			SELECT id FROM voice_workout_utterances
			WHERE session_id = $1 ORDER BY said_at DESC, created_at DESC LIMIT 1
		)
	`, sessionID, encoded, parseError)
	return err
}

// generateTitle names the workout from the exercises that were actually logged.
// An empty result is fine: the caller keeps whatever title the session already
// had rather than inventing one.
func (h *VoiceWorkoutHandler) generateTitle(ctx context.Context, sessionID string) string {
	if h.ai == nil {
		return ""
	}

	draft, err := h.loadDraft(ctx, sessionID)
	if err != nil || len(draft) == 0 {
		return ""
	}

	titles := make([]string, 0, len(draft))
	for _, exercise := range draft {
		titles = append(titles, exercise.Title)
	}

	messages := []ChatMessage{
		{Role: "system", Content: "Придумай короткое название силовой тренировки по списку упражнений. " +
			"Максимум три слова, без кавычек, без точки, без пояснений. " +
			"Ориентируйся на основные задействованные группы мышц, например \"Спина и бицепс\" или \"Push-день\". " +
			"Верни только название."},
		{Role: "user", Content: strings.Join(titles, ", ")},
	}

	answer, err := h.ai.CompleteWithModel(ctx, "voice_workout_title", messages, h.parseModel, h.parseEffort)
	if err != nil {
		h.logger.Warn().Err(err).Str("session_id", sessionID).Msg("generate workout title")
		return ""
	}
	return sanitizeWorkoutTitle(answer)
}

// sanitizeWorkoutTitle trims a model answer down to something that fits the
// column and reads like a title, however chatty the answer was.
func sanitizeWorkoutTitle(answer string) string {
	title := strings.TrimSpace(answer)
	if line, _, found := strings.Cut(title, "\n"); found {
		title = strings.TrimSpace(line)
	}
	title = strings.Trim(title, "\"'`«».:;")
	title = strings.Join(strings.Fields(title), " ")

	const limit = 120
	if len(title) > limit {
		trimmed := title[:limit]
		for len(trimmed) > 0 && !utf8.ValidString(trimmed) {
			trimmed = trimmed[:len(trimmed)-1]
		}
		title = strings.TrimSpace(trimmed)
	}
	return title
}
