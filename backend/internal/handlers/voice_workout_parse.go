package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
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
	Times    int              `json:"times_logged"`
	LastSets []voiceParsedSet `json:"-"`
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
	Entries   []voiceParsedEntry    `json:"entries"`
	Task      *voiceParsedTask      `json:"task"`
	Unmatched []string              `json:"unmatched"`
}

// voiceParsedTask is a task dictated into the dashboard. The deadline comes back
// already resolved to a timestamp: "в пятницу" can only be turned into a date
// against the moment the phrase was said, which the prompt carries.
type voiceParsedTask struct {
	Title string `json:"title"`
	// Project is copied verbatim from the list in the prompt, the same rule the
	// workout parse uses for template ids: an invented project cannot be filed.
	Project     string             `json:"project"`
	Description string             `json:"description"`
	DueAt       string             `json:"due_at"`
	Priority    int                `json:"priority"`
	Repeat      *voiceParsedRepeat `json:"repeat"`
}

// voiceParsedRepeat is a repeat rule in the words people actually say: every N
// days, weeks or months. Turning it into what the provider stores is this side's
// job, not the model's arithmetic.
type voiceParsedRepeat struct {
	Every int    `json:"every"`
	Unit  string `json:"unit"`
}

// voiceInterpretation is the router's verdict on a phrase. Parsed stays nil when
// nothing usable came back, which is what tells the caller not to touch the draft.
type voiceInterpretation struct {
	Domain    string
	Parsed    []voiceParsedExercise
	Entries   []voiceParsedEntry
	Foods     []voiceFoodCandidate
	Task      *voiceParsedTask
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := h.loadCandidateLastSets(ctx, userID, candidates); err != nil {
		return nil, fmt.Errorf("load last exercise sets: %w", err)
	}
	return candidates, nil
}

// loadCandidateLastSets attaches the sets from the most recent workout that
// contains each exercise. They are deterministic fallback values for a phrase
// such as "два подхода жима как обычно" when weight or reps are omitted.
func (h *VoiceWorkoutHandler) loadCandidateLastSets(ctx context.Context, userID string, candidates []voiceExerciseCandidate) error {
	rows, err := h.db.Query(ctx, `
		WITH ranked AS (
			SELECT lower(ws.exercise_name) AS exercise_key,
			       COALESCE(ws.set_type, 'normal') AS set_type,
			       ws.reps, ws.weight_kg::double precision, ws.duration_seconds,
			       ws.set_index,
			       DENSE_RANK() OVER (
				   PARTITION BY lower(ws.exercise_name)
				   ORDER BY w.started_at DESC, w.id DESC
			       ) AS workout_rank
			FROM workout_sets ws
			JOIN workouts w ON w.id = ws.workout_id
			WHERE w.user_id = $1
		)
		SELECT exercise_key, set_type, reps, weight_kg, duration_seconds
		FROM ranked
		WHERE workout_rank = 1
		ORDER BY exercise_key, set_index
	`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	byTitle := make(map[string]*voiceExerciseCandidate, len(candidates))
	for i := range candidates {
		byTitle[strings.ToLower(candidates[i].Title)] = &candidates[i]
	}
	for rows.Next() {
		var key string
		var set voiceParsedSet
		if err := rows.Scan(&key, &set.Type, &set.Reps, &set.WeightKg, &set.DurationSeconds); err != nil {
			return err
		}
		if candidate := byTitle[key]; candidate != nil {
			candidate.LastSets = append(candidate.LastSets, set)
		}
	}
	return rows.Err()
}

// buildVoiceParsePrompt states the rules the dictated phrasing actually needs.
// Every rule here comes from a real phrasing rather than from imagination, and
// the hard constraint is that a template_id must be copied from the list: an
// invented id would be rejected by Hevy, or worse, accepted as another exercise.
func buildVoiceParsePrompt(candidates []voiceExerciseCandidate, foods []voiceFoodCandidate, projects []taskProject, draft []voiceParsedExercise, workoutOpen bool, now time.Time) string {
	var sb strings.Builder
	sb.WriteString("Ты разбираешь фразу, надиктованную вслух по-русски в дневник пользователя.\n")
	sb.WriteString("Верни только JSON, без markdown и без пояснений, в формате:\n")
	sb.WriteString(`{"domain":"workout","exercises":[{"template_id":"...","title":"...","sets":[{"type":"normal","reps":5,"weight_kg":13.5}]}],"entries":[],"unmatched":["..."]}`)
	sb.WriteString("\nДля еды формат такой:\n")
	sb.WriteString(`{"domain":"food","exercises":[],"entries":[{"food_id":"...","serving_id":"...","name":"...","grams":70,"meal":"breakfast"}],"unmatched":["..."]}`)
	sb.WriteString("\nДля задачи формат такой:\n")
	sb.WriteString(`{"domain":"task","exercises":[],"entries":[],"task":{"title":"забрать запчасти","project":"citroen","description":"","due_at":"2026-09-05T12:00:00+03:00","priority":0,"repeat":null},"unmatched":[]}`)
	sb.WriteString("\n\nСначала определи domain - о чём фраза:\n")
	sb.WriteString("- workout: упражнения, подходы, повторения, веса.\n")
	sb.WriteString("- food: съеденное, продукты, граммы, калории.\n")
	sb.WriteString("- task: то, что надо сделать: поручение себе, дело, напоминание, покупка.\n")
	sb.WriteString("- weight: собственный вес пользователя.\n")
	sb.WriteString("- note: всё остальное, мысли и заметки.\n")
	sb.WriteString("Если domain не workout, не food и не task, верни только domain, а массивы оставь пустыми.\n")
	sb.WriteString("\nПравила разбора задачи:\n")
	sb.WriteString("- title - это само дело в инфинитиве, без слов \"надо\", \"не забыть\" и \"напомни\".\n")
	sb.WriteString(fmt.Sprintf("- Сейчас %s. Относительный срок (\"завтра\", \"в пятницу\", \"через неделю\") переведи в due_at по этому времени и в этом же часовом поясе.\n", now.Format("2006-01-02 15:04 -07:00, Monday")))
	sb.WriteString("- Если время дня не названо, поставь 12:00. Если срока нет вообще, оставь due_at пустой строкой - выдуманный дедлайн хуже, чем его отсутствие.\n")
	sb.WriteString("- priority заполняй только когда срочность сказана словами: 1 низкий, 2 средний, 3 высокий, 4 срочно. Иначе 0.\n")
	sb.WriteString("- description заполняй только если во фразе есть подробности сверх названия: адрес, номер, условие. Пересказывать title в description не надо.\n")
	sb.WriteString("- repeat заполняй только если сказано про повторение: \"каждый день\", \"раз в две недели\", \"каждый месяц\". unit - day, week или month, every - число. Иначе null.\n")
	sb.WriteString("- \"каждую пятницу\" - это repeat {\"every\":1,\"unit\":\"week\"}, а ближайшую пятницу положи в due_at: интервал повтора считается от срока.\n")
	sb.WriteString("- Прошедшее дело - это не задача: \"купил хлеб\" не task.\n")
	if len(projects) > 0 {
		sb.WriteString("- project: скопируй строку из списка ниже целиком, если проект назван вслух или однозначно следует из дела. Не придумывай новых названий: если ничего не подходит, оставь пустую строку и задача уйдёт в проект по умолчанию.\n")
		sb.WriteString("\nПроекты пользователя:\n")
		for _, project := range projects {
			line := "- " + project.Path
			if project.IsDefault {
				line += " (по умолчанию)"
			}
			sb.WriteString(line + "\n")
		}
	}
	if workoutOpen {
		sb.WriteString("Сейчас у пользователя открыта тренировка. Короткая фраза с числами почти наверняка workout: \"ещё 8\" или \"12 по 30\" - это подходы.\n")
		sb.WriteString("Если во фразе есть вес, повторения или что-то похожее на название упражнения - это workout, даже если распознавание исказило слова до бессмыслицы. Тогда положи исходную формулировку в unmatched, но domain оставь workout: это честнее, чем объявить надиктованный подход заметкой.\n")
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
	sb.WriteString("- Если упражнение распознано, но вес или повторения не названы (например, сказано \"как обычно\"), всё равно верни упражнение и нужное количество подходов, оставив неназванные поля пустыми. Backend подставит их из последней тренировки. Не клади такую фразу в unmatched только из-за отсутствующих чисел.\n")
	sb.WriteString("- Перед ответом разбей составную фразу на отдельные упоминания упражнений. Каждое упоминание обязано быть представлено ровно один раз: либо объектом в exercises, либо исходным фрагментом в unmatched. Никогда не пропускай начало, середину или конец фразы молча.\n")
	sb.WriteString("- Общее название мышцы вместе с \"как обычно\" означает привычное упражнение пользователя: выбери наиболее часто выполняемый подходящий вариант из списка по times_logged. Например, \"на бицепс делал как обычно\" должно стать самым частым упражнением на бицепс с одним подходом и пустыми неназванными reps/weight_kg.\n")

	if len(draft) > 0 {
		sb.WriteString("\nВ этой тренировке уже записано (не дублируй, только добавляй новое из фразы):\n")
		for _, exercise := range draft {
			sb.WriteString(fmt.Sprintf("- %s: %d подходов\n", exercise.Title, len(exercise.Sets)))
		}
	}

	sb.WriteString("\nПравила разбора еды:\n")
	sb.WriteString("- food_id и serving_id обязаны быть скопированы одной парой из списка продуктов ниже. Не придумывай и не смешивай пары от разных продуктов.\n")
	sb.WriteString("- Продукта нет в списке - не подбирай похожий, положи формулировку в unmatched.\n")
	sb.WriteString("- Если пользователь назвал вес в граммах, скопируй число без пересчёта в grams. Например, \"70 г\" это grams = 70. Backend сам переведёт граммы в units.\n")
	sb.WriteString("- units указывай только когда пользователь назвал именно порции или штуки: \"полторы порции\" это units = 1.5. Не записывай граммы в units.\n")
	sb.WriteString("- Количество не названо - не указывай ни grams, ни units, подставится обычное количество пользователя.\n")
	sb.WriteString("- meal указывай только если пользователь назвал приём пищи словами (завтрак, обед, ужин, перекус). Иначе оставь пустым: он определится по времени суток.\n")
	sb.WriteString("- Несколько продуктов в одной фразе - несколько записей в entries.\n")

	sb.WriteString("\nДоступные упражнения (times_logged - как часто пользователь их делает, это лучший критерий при неоднозначности):\n")
	if encoded, err := json.Marshal(candidates); err == nil {
		sb.Write(encoded)
	}
	sb.WriteString("\n\nДоступные продукты (rank - чем меньше, тем чаще пользователь это ест; usual_units - его обычное количество):\n")
	if encoded, err := json.Marshal(foods); err == nil {
		sb.Write(encoded)
	}
	sb.WriteString("\n")
	return sb.String()
}

// parsePhrase asks the model to turn one phrase into exercises and sets.
func (h *VoiceWorkoutHandler) parsePhrase(ctx context.Context, userID, text string, draft []voiceParsedExercise, workoutOpen bool) (voiceParseResult, []voiceExerciseCandidate, []voiceFoodCandidate, error) {
	var result voiceParseResult

	candidates, err := h.loadCandidates(ctx, userID)
	if err != nil {
		return result, nil, nil, err
	}
	if len(candidates) == 0 {
		return result, nil, nil, fmt.Errorf("no exercise candidates: sync Hevy first")
	}

	// Both catalogues travel in the one call. Classifying first and parsing second
	// would double the wait of someone standing at a machine, and the extra input
	// costs a fraction of a kopeck on the parse model.
	foods, err := h.loadFoodCandidates(ctx, userID)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load food candidates")
	}

	// Projects come from the local mirror the sync keeps, so naming a project
	// out loud costs no round trip to the provider.
	projects, err := h.loadTaskProjects(ctx, userID)
	if err != nil {
		h.logger.Warn().Err(err).Msg("load task projects")
	}

	messages := []ChatMessage{
		{Role: "system", Content: buildVoiceParsePrompt(candidates, foods, projects, draft, workoutOpen, time.Now())},
		{Role: "user", Content: text},
	}

	// Parsing is extraction, not reasoning. On the default checkup model the same
	// phrase took 43 seconds and 2 rubles against 5 seconds and 9 kopecks on the
	// fast one, and 43 seconds standing at a machine is not usable.
	answer, err := h.ai.CompleteWithModel(ctx, "voice_workout_parse", messages, h.parseModel, h.parseEffort)
	if err != nil {
		return result, candidates, foods, err
	}

	parsed, err := decodeVoiceParseResult(answer)
	if err != nil {
		return result, candidates, foods, err
	}
	return parsed, candidates, foods, nil
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

	parsed, candidates, foods, err := h.parsePhrase(ctx, userID, text, draft, workoutOpen)
	if err != nil {
		h.logger.Warn().Err(err).Msg("parse phrase")
		response.ParseError = err.Error()
		return voiceInterpretation{Domain: resolveVoiceDomain("", workoutOpen)}
	}

	domain := resolveVoiceDomain(parsed.Domain, workoutOpen)
	if domain == voiceDomainFood {
		return voiceInterpretation{
			Domain:    domain,
			Entries:   parsed.Entries,
			Foods:     foods,
			Unmatched: parsed.Unmatched,
		}
	}
	if domain == voiceDomainTask {
		return voiceInterpretation{
			Domain:    domain,
			Task:      parsed.Task,
			Unmatched: parsed.Unmatched,
		}
	}
	if domain != voiceDomainWorkout {
		return voiceInterpretation{Domain: domain}
	}

	withHistory := fillMissingSetMetrics(parsed.Exercises, candidates)
	kept, rejected := validateParsedExercises(withHistory, candidates)
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
