package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

func vInt(v int) *int        { return &v }
func vKg(v float64) *float64 { return &v }

var voiceTestCandidates = []voiceExerciseCandidate{
	{TemplateID: "422B08F1", Title: "Lateral Raise (Dumbbell)", Type: "weight_reps", Times: 25},
	{TemplateID: "1B2B1E7C", Title: "Pull Up", Type: "reps_only", Times: 11},
	{TemplateID: "6FCD7755", Title: "Chest Dip", Type: "bodyweight_weighted", Times: 10},
	{TemplateID: "B60A678F", Title: "Swimming", Type: "duration", Times: 6},
}

func TestValidateRejectsInventedTemplateID(t *testing.T) {
	parsed := []voiceParsedExercise{
		{TemplateID: "DEADBEEF", Title: "Neck Curl", Sets: []voiceParsedSet{{Type: "normal", Reps: vInt(10)}}},
	}

	kept, rejected := validateParsedExercises(parsed, voiceTestCandidates)

	if len(kept) != 0 {
		t.Fatalf("kept an exercise with an invented id: %+v", kept)
	}
	if len(rejected) != 1 || !strings.Contains(rejected[0], "Neck Curl") {
		t.Fatalf("rejected = %v", rejected)
	}
}

func TestValidateDropsWeightForRepsOnlyExercise(t *testing.T) {
	// "5 подтягиваний" with a weight the model should never have attached.
	parsed := []voiceParsedExercise{
		{TemplateID: "1B2B1E7C", Title: "Pull Up", Sets: []voiceParsedSet{
			{Type: "normal", Reps: vInt(5), WeightKg: vKg(80)},
			{Type: "normal", Reps: vInt(4)},
		}},
	}

	kept, rejected := validateParsedExercises(parsed, voiceTestCandidates)

	if len(rejected) != 0 {
		t.Fatalf("unexpected rejections: %v", rejected)
	}
	if len(kept) != 1 || len(kept[0].Sets) != 2 {
		t.Fatalf("kept = %+v", kept)
	}
	if kept[0].Sets[0].WeightKg != nil {
		t.Fatalf("weight survived on a reps_only exercise: %v", *kept[0].Sets[0].WeightKg)
	}
	if kept[0].Sets[0].Reps == nil || *kept[0].Sets[0].Reps != 5 {
		t.Fatalf("reps lost: %+v", kept[0].Sets[0])
	}
}

func TestValidateKeepsWeightForWeightedBodyweightExercise(t *testing.T) {
	parsed := []voiceParsedExercise{
		{TemplateID: "6FCD7755", Title: "Chest Dip", Sets: []voiceParsedSet{
			{Type: "normal", Reps: vInt(20), WeightKg: vKg(10)},
		}},
	}

	kept, _ := validateParsedExercises(parsed, voiceTestCandidates)
	if len(kept) != 1 || kept[0].Sets[0].WeightKg == nil || *kept[0].Sets[0].WeightKg != 10 {
		t.Fatalf("added weight was dropped: %+v", kept)
	}
}

func TestValidateRejectsMisheardWeight(t *testing.T) {
	// 13.5 heard as 1350: the bound is what stops it entering the history.
	parsed := []voiceParsedExercise{
		{TemplateID: "422B08F1", Title: "Lateral Raise (Dumbbell)", Sets: []voiceParsedSet{
			{Type: "normal", Reps: vInt(12), WeightKg: vKg(1350)},
			{Type: "normal", Reps: vInt(12), WeightKg: vKg(13.5)},
		}},
	}

	kept, _ := validateParsedExercises(parsed, voiceTestCandidates)
	if len(kept) != 1 || len(kept[0].Sets) != 1 {
		t.Fatalf("expected only the plausible set to survive, got %+v", kept)
	}
	if *kept[0].Sets[0].WeightKg != 13.5 {
		t.Fatalf("wrong set survived: %+v", kept[0].Sets[0])
	}
}

func TestValidateRejectsExerciseLeftWithoutSets(t *testing.T) {
	parsed := []voiceParsedExercise{
		{TemplateID: "422B08F1", Title: "Lateral Raise (Dumbbell)", Sets: []voiceParsedSet{
			{Type: "normal", Reps: vInt(0)},
		}},
	}

	kept, rejected := validateParsedExercises(parsed, voiceTestCandidates)
	if len(kept) != 0 {
		t.Fatalf("kept an exercise with no usable sets: %+v", kept)
	}
	if len(rejected) != 1 || !strings.Contains(rejected[0], "подходы") {
		t.Fatalf("rejected = %v", rejected)
	}
}

func TestValidateNormalizesUnknownSetType(t *testing.T) {
	parsed := []voiceParsedExercise{
		{TemplateID: "1B2B1E7C", Sets: []voiceParsedSet{{Type: "разминочный", Reps: vInt(8)}}},
	}

	kept, _ := validateParsedExercises(parsed, voiceTestCandidates)
	if len(kept) != 1 || kept[0].Sets[0].Type != "normal" {
		t.Fatalf("set type = %+v", kept)
	}
	// The canonical Hevy title has to replace whatever the model wrote.
	if kept[0].Title != "Pull Up" {
		t.Fatalf("title = %q, want Pull Up", kept[0].Title)
	}
}

func TestValidateUsesDurationForDurationExercise(t *testing.T) {
	parsed := []voiceParsedExercise{
		{TemplateID: "B60A678F", Title: "Swimming", Sets: []voiceParsedSet{
			{Type: "normal", Reps: vInt(30), DurationSeconds: vInt(1800)},
		}},
	}

	kept, _ := validateParsedExercises(parsed, voiceTestCandidates)
	if len(kept) != 1 {
		t.Fatalf("kept = %+v", kept)
	}
	if kept[0].Sets[0].Reps != nil {
		t.Fatal("reps survived on a duration exercise")
	}
	if kept[0].Sets[0].DurationSeconds == nil || *kept[0].Sets[0].DurationSeconds != 1800 {
		t.Fatalf("duration lost: %+v", kept[0].Sets[0])
	}
}

func TestMergeAppendsSetsToTheSameExercise(t *testing.T) {
	draft := []voiceParsedExercise{
		{TemplateID: "422B08F1", Title: "Lateral Raise (Dumbbell)", Sets: []voiceParsedSet{
			{Type: "normal", Reps: vInt(12), WeightKg: vKg(13.5)},
		}},
	}
	addition := []voiceParsedExercise{
		{TemplateID: "422B08F1", Title: "Lateral Raise (Dumbbell)", Sets: []voiceParsedSet{
			{Type: "normal", Reps: vInt(11), WeightKg: vKg(13.5)},
		}},
		{TemplateID: "1B2B1E7C", Title: "Pull Up", Sets: []voiceParsedSet{
			{Type: "normal", Reps: vInt(5)},
		}},
	}

	merged := mergeVoiceDraft(draft, addition)

	if len(merged) != 2 {
		t.Fatalf("expected 2 exercises, got %d: %+v", len(merged), merged)
	}
	if len(merged[0].Sets) != 2 {
		t.Fatalf("sets were not appended: %+v", merged[0])
	}
	if merged[1].Title != "Pull Up" {
		t.Fatalf("second exercise = %+v", merged[1])
	}
	// The original draft must not be mutated: the caller keeps it for comparison.
	if len(draft[0].Sets) != 1 {
		t.Fatalf("merge mutated the input draft: %+v", draft[0])
	}
}

func TestSummarizeDraftShowsNumbers(t *testing.T) {
	draft := []voiceParsedExercise{
		{TemplateID: "422B08F1", Title: "Lateral Raise (Dumbbell)", Sets: []voiceParsedSet{
			{Type: "normal", Reps: vInt(12), WeightKg: vKg(13.5)},
			{Type: "failure", Reps: vInt(9), WeightKg: vKg(13.5)},
		}},
		{TemplateID: "1B2B1E7C", Title: "Pull Up", Sets: []voiceParsedSet{{Type: "normal", Reps: vInt(5)}}},
	}

	summary := summarizeVoiceDraft(draft)

	for _, want := range []string{"Lateral Raise (Dumbbell): 12×13.5кг", "9×13.5кг (failure)", "Pull Up: 5"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestDecodeParseResultStripsMarkdownFence(t *testing.T) {
	answer := "```json\n{\"exercises\":[{\"template_id\":\"1B2B1E7C\",\"title\":\"Pull Up\",\"sets\":[{\"type\":\"normal\",\"reps\":5}]}],\"unmatched\":[]}\n```"

	result, err := decodeVoiceParseResult(answer)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Exercises) != 1 || result.Exercises[0].TemplateID != "1B2B1E7C" {
		t.Fatalf("result = %+v", result)
	}
}

func TestDecodeParseResultHandlesLeadingProse(t *testing.T) {
	answer := "Вот разбор:\n{\"exercises\":[],\"unmatched\":[\"непонятная фраза\"]}"

	result, err := decodeVoiceParseResult(answer)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Unmatched) != 1 {
		t.Fatalf("unmatched = %v", result.Unmatched)
	}
}

func TestValidateReportsPartiallyDroppedSets(t *testing.T) {
	// What deepseek-v4-flash actually produced for "11 раз, 2 подхода": the
	// second set came back with reps 0. Dropping it quietly would log one set
	// where two were performed, and the set count is exactly what cannot be
	// checked afterwards.
	parsed := []voiceParsedExercise{
		{TemplateID: "422B08F1", Title: "Lateral Raise (Dumbbell)", Sets: []voiceParsedSet{
			{Type: "normal", Reps: vInt(11), WeightKg: vKg(16)},
			{Type: "normal", Reps: vInt(0), WeightKg: vKg(16)},
		}},
	}

	kept, rejected := validateParsedExercises(parsed, voiceTestCandidates)

	if len(kept) != 1 || len(kept[0].Sets) != 1 {
		t.Fatalf("kept = %+v", kept)
	}
	if len(rejected) != 1 {
		t.Fatalf("a dropped set was not reported: %v", rejected)
	}
	if !strings.Contains(rejected[0], "Lateral Raise (Dumbbell)") || !strings.Contains(rejected[0], "1 подход") {
		t.Fatalf("report does not name what was lost: %q", rejected[0])
	}
}

func TestDecodeSetAcceptsCommaDecimalWeight(t *testing.T) {
	// Real dictation: "Lateral Rises 13,5 килограмм 10 раз". iOS writes the
	// Russian decimal comma, and a model may pass it through as a quoted string.
	// Strict decoding would fail the whole phrase over this one field.
	cases := map[string]float64{
		`{"type":"normal","reps":10,"weight_kg":13.5}`:     13.5,
		`{"type":"normal","reps":10,"weight_kg":"13.5"}`:   13.5,
		`{"type":"normal","reps":10,"weight_kg":"13,5"}`:   13.5,
		`{"type":"normal","reps":10,"weight_kg":" 13,5 "}`: 13.5,
	}
	for payload, want := range cases {
		var set voiceParsedSet
		if err := json.Unmarshal([]byte(payload), &set); err != nil {
			t.Fatalf("unmarshal %s: %v", payload, err)
		}
		if set.WeightKg == nil || *set.WeightKg != want {
			t.Fatalf("%s -> %v, want %v", payload, set.WeightKg, want)
		}
		if set.Reps == nil || *set.Reps != 10 {
			t.Fatalf("%s lost reps: %v", payload, set.Reps)
		}
	}
}

func TestDecodeSetAcceptsQuotedAndFloatReps(t *testing.T) {
	cases := map[string]int{
		`{"reps":"12"}`:   12,
		`{"reps":12.0}`:   12,
		`{"reps":"12.0"}`: 12,
	}
	for payload, want := range cases {
		var set voiceParsedSet
		if err := json.Unmarshal([]byte(payload), &set); err != nil {
			t.Fatalf("unmarshal %s: %v", payload, err)
		}
		if set.Reps == nil || *set.Reps != want {
			t.Fatalf("%s -> %v, want %d", payload, set.Reps, want)
		}
	}
}

func TestDecodeSetKeepsAbsentFieldsAbsent(t *testing.T) {
	// Absence and zero mean different things: a reps_only exercise has no weight
	// at all, and turning that into 0 kg would be a claim nobody made.
	for _, payload := range []string{
		`{"type":"normal","reps":5}`,
		`{"type":"normal","reps":5,"weight_kg":null}`,
		`{"type":"normal","reps":5,"weight_kg":""}`,
		`{"type":"normal","reps":5,"weight_kg":"около двадцати"}`,
	} {
		var set voiceParsedSet
		if err := json.Unmarshal([]byte(payload), &set); err != nil {
			t.Fatalf("unmarshal %s: %v", payload, err)
		}
		if set.WeightKg != nil {
			t.Fatalf("%s produced a weight of %v", payload, *set.WeightKg)
		}
		if set.Reps == nil || *set.Reps != 5 {
			t.Fatalf("%s lost reps: %v", payload, set.Reps)
		}
	}
}

func TestDecodeParseResultSurvivesOneBadField(t *testing.T) {
	// The point of leniency: a single unreadable field must not cost the sets that
	// were understood.
	answer := `{"domain":"workout","exercises":[{"template_id":"422B08F1","title":"Lateral Raise (Dumbbell)",
	  "sets":[{"type":"normal","reps":12,"weight_kg":"13,5"},{"type":"normal","reps":12,"weight_kg":"хз"}]}],"unmatched":[]}`

	result, err := decodeVoiceParseResult(answer)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Exercises) != 1 || len(result.Exercises[0].Sets) != 2 {
		t.Fatalf("result = %+v", result)
	}

	kept, _ := validateParsedExercises(result.Exercises, voiceTestCandidates)
	if len(kept) != 1 || len(kept[0].Sets) != 2 {
		t.Fatalf("kept = %+v", kept)
	}
	if kept[0].Sets[0].WeightKg == nil || *kept[0].Sets[0].WeightKg != 13.5 {
		t.Fatalf("first set weight = %v", kept[0].Sets[0].WeightKg)
	}
	// The unreadable weight is dropped, but the twelve reps survive.
	if kept[0].Sets[1].WeightKg != nil {
		t.Fatalf("second set kept a bogus weight: %v", *kept[0].Sets[1].WeightKg)
	}
	if kept[0].Sets[1].Reps == nil || *kept[0].Sets[1].Reps != 12 {
		t.Fatalf("second set lost reps: %v", kept[0].Sets[1].Reps)
	}
}
