// Package session owns the login cookie: how long it lives and when it renews.
//
// It is its own package because two callers need the same rules: the login
// handler, which mints the cookie, and the auth middleware, which renews it on
// activity. Before this existed the token was only ever issued at login, so
// every session died exactly TTL after logging in no matter how much the
// account was used, which read as being logged out at random.
package session

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// CookieName is the cookie the token travels in.
const CookieName = "token"

const (
	// TTL is how long a freshly issued token stays valid.
	TTL = 7 * 24 * time.Hour
	// MaxLifetime caps renewal against the original login rather than the
	// current token, so a stolen cookie cannot be renewed indefinitely. Past
	// this the account has to log in again.
	MaxLifetime = 180 * 24 * time.Hour
	// renewAfter is how much of the TTL must be spent before a request renews
	// the cookie. Renewing on every request would re-sign the token and rewrite
	// the header on every single call for no gain.
	renewAfter = TTL / 3
)

// Issue signs a token for the user and writes the cookie. authTime is when the
// account actually logged in with a password; it is carried through every later
// renewal, because MaxLifetime is measured from it.
func Issue(w http.ResponseWriter, r *http.Request, secret, userID, username string, authTime time.Time) error {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":       userID,
		"username":  username,
		"iat":       now.Unix(),
		"exp":       now.Add(TTL).Unix(),
		"auth_time": authTime.Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    signed,
		Path:     "/",
		MaxAge:   int(TTL / time.Second),
		HttpOnly: true,
		Secure:   SecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// Renew re-issues the cookie for an already validated token once it is far
// enough through its life, and reports whether it wrote one. Every reason to
// skip is silent: a request must still succeed when its session cannot be
// extended, it just gets no fresh cookie.
func Renew(w http.ResponseWriter, r *http.Request, secret string, claims jwt.MapClaims) bool {
	now := time.Now()

	exp, ok := claimTime(claims, "exp")
	if !ok || now.Add(TTL-renewAfter).Before(exp) {
		return false
	}

	// Tokens minted before auth_time existed only carry iat, which for those is
	// the login time anyway.
	authTime, ok := claimTime(claims, "auth_time")
	if !ok {
		authTime, ok = claimTime(claims, "iat")
	}
	if !ok || now.Sub(authTime) > MaxLifetime {
		return false
	}

	userID, _ := claims["sub"].(string)
	username, _ := claims["username"].(string)
	if userID == "" {
		return false
	}
	return Issue(w, r, secret, userID, username, authTime) == nil
}

// Clear expires the cookie.
func Clear(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   SecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// SecureRequest reports whether the request reached us over TLS, directly or
// through the ingress.
func SecureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func claimTime(claims jwt.MapClaims, name string) (time.Time, bool) {
	switch value := claims[name].(type) {
	case float64:
		return time.Unix(int64(value), 0), true
	case int64:
		return time.Unix(value, 0), true
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return time.Unix(parsed, 0), true
		}
	}
	return time.Time{}, false
}
