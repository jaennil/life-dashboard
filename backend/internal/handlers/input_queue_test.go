package handlers

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestInputNotificationBodyOmitsTranscript(t *testing.T) {
	tests := []struct {
		name    string
		display string
		want    string
	}{
		{
			name:    "dictated input",
			display: "Услышал: жим над головой\nShoulder Press (Dumbbell): 1×11",
			want:    "Shoulder Press (Dumbbell): 1×11",
		},
		{
			name:    "typed input",
			display: "Введено: лимонад с витаминами\nЗаписал в дневник.",
			want:    "Записал в дневник.",
		},
		{
			name:    "failure without transcript",
			display: "Не удалось обработать: ai unavailable",
			want:    "Не удалось обработать: ai unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inputNotificationBody(tt.display); got != tt.want {
				t.Fatalf("inputNotificationBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInputJobRetryDelaySeparatesStallsFromOutages(t *testing.T) {
	// A provider that answered with an error gets the long wait.
	if got := inputJobRetryDelay(1, errAIUpstream); got != time.Hour {
		t.Errorf("upstream error retried in %s, want 1h", got)
	}
	if got := inputJobRetryDelay(2, fmt.Errorf("parse: %w", errAIUpstream)); got != 3*time.Hour {
		t.Errorf("second upstream error retried in %s, want 3h", got)
	}

	// A provider that never answered gets asked again while the phone is still
	// in hand: this is the failure that left a dictated set hanging for an hour.
	if got := inputJobRetryDelay(1, errAIUnavailable); got != time.Minute {
		t.Errorf("stall retried in %s, want 1m", got)
	}
	if got := inputJobRetryDelay(2, context.DeadlineExceeded); got != 5*time.Minute {
		t.Errorf("second stall retried in %s, want 5m", got)
	}
	// A model that answered with unusable JSON is worth asking again at once too.
	if got := inputJobRetryDelay(1, errors.New("decode parse result: unexpected end of JSON input")); got != time.Minute {
		t.Errorf("decode failure retried in %s, want 1m", got)
	}
}

func TestInputJobRetryDelayStaysInsideTheTable(t *testing.T) {
	// attempts is whatever the database says, so the lookup must not depend on it
	// being in range.
	for _, attempts := range []int{0, 1, 2, 3, 99} {
		if got := inputJobRetryDelay(attempts, errAIUpstream); got <= 0 {
			t.Fatalf("attempts=%d gave %s", attempts, got)
		}
	}
}
