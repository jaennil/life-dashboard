package handlers

import "testing"

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
