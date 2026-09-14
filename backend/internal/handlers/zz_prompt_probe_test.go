package handlers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Temporary harness: builds the real parse prompt over the real catalogue and
// asks the real parse model, so a refusal can be read instead of guessed at.
func TestProbePrompt(t *testing.T) {
	key := os.Getenv("AI_KEY")
	dump := os.Getenv("SHORTLIST_DUMP")
	if key == "" || dump == "" {
		t.Skip("no key or dump")
	}

	file, err := os.Open(dump)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	grams := 100.0
	kcal := 150.0
	units := 1.0
	var foods []voiceFoodCandidate
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "|", 2)
		if len(parts) != 2 {
			continue
		}
		foods = append(foods, voiceFoodCandidate{
			FoodID: parts[0], ServingID: "s", Name: parts[1],
			ServingDescription: "1 :custom:100г, 100 g " + parts[1],
			ServingGrams:       &grams, CaloriesPerServing: &kcal, UsualUnits: &units,
		})
	}

	phrase := os.Getenv("PROBE_PHRASE")
	shortlist := rankFoodCandidatesForPhrase(phrase, foods, voiceFoodCandidateLimit)
	for i, candidate := range shortlist[:6] {
		t.Logf("список %d: %s", i+1, candidate.Name)
	}
	exercises := []voiceExerciseCandidate{{TemplateID: "t1", Title: "Жим лёжа", Type: "weight_reps"}}
	prompt := buildVoiceParsePrompt(exercises, shortlist, nil, nil, false, time.Now())

	body, err := json.Marshal(map[string]any{
		"model": os.Getenv("PROBE_MODEL"),
		"messages": []map[string]string{
			{"role": "system", "content": prompt},
			{"role": "user", "content": phrase},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.aitunnel.ru/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded.Choices) == 0 {
		t.Fatalf("статус %d, ответ: %s", resp.StatusCode, string(raw)[:min(len(string(raw)), 400)])
	}
	t.Logf("фраза: %s", phrase)
	t.Logf("ответ модели: %s", strings.TrimSpace(decoded.Choices[0].Message.Content))
}
