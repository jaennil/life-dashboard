package middleware

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"life-dashboard/internal/syncstate"
)

func TrackActivity(db *pgxpool.Pool, logger zerolog.Logger) func(http.Handler) http.Handler {
	log := logger.With().Str("middleware", "activity").Logger()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if userID, ok := r.Context().Value(UserIDKey).(string); ok && userID != "" {
				if err := syncstate.TouchUserActivity(r.Context(), db, userID); err != nil {
					log.Warn().Err(err).Str("user_id", userID).Msg("touch user activity")
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
