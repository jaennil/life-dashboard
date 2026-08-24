package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret"

func claimsFor(t *testing.T, authTime, issued time.Time, ttl time.Duration, omitAuthTime bool) jwt.MapClaims {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":      "user-1",
		"username": "jaennil",
		"iat":      float64(issued.Unix()),
		"exp":      float64(issued.Add(ttl).Unix()),
	}
	if !omitAuthTime {
		claims["auth_time"] = float64(authTime.Unix())
	}
	return claims
}

func renewedCookie(t *testing.T, claims jwt.MapClaims) (*http.Cookie, bool) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	renewed := Renew(rec, req, testSecret, claims)

	cookies := rec.Result().Cookies()
	if !renewed {
		if len(cookies) != 0 {
			t.Fatalf("skipped renewal but still wrote %d cookie(s)", len(cookies))
		}
		return nil, false
	}
	if len(cookies) != 1 {
		t.Fatalf("renewed but wrote %d cookies, want 1", len(cookies))
	}
	return cookies[0], true
}

func parse(t *testing.T, raw string) jwt.MapClaims {
	t.Helper()
	token, err := jwt.Parse(raw, func(*jwt.Token) (interface{}, error) { return []byte(testSecret), nil })
	if err != nil {
		t.Fatalf("parse renewed token: %v", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("renewed token has no map claims")
	}
	return claims
}

func TestRenewSkipsFreshToken(t *testing.T) {
	now := time.Now()
	// Just issued, so nothing of the TTL has been spent yet.
	if _, renewed := renewedCookie(t, claimsFor(t, now, now, TTL, false)); renewed {
		t.Fatal("renewed a token that was just issued")
	}
}

func TestRenewExtendsUsedToken(t *testing.T) {
	now := time.Now()
	issued := now.Add(-(renewAfter + time.Hour))
	cookie, renewed := renewedCookie(t, claimsFor(t, issued, issued, TTL, false))
	if !renewed {
		t.Fatal("did not renew a token past the renewal window")
	}

	claims := parse(t, cookie.Value)
	exp, ok := claimTime(claims, "exp")
	if !ok {
		t.Fatal("renewed token has no exp")
	}
	if remaining := time.Until(exp); remaining < TTL-time.Minute {
		t.Fatalf("renewed token expires in %s, want about %s", remaining, TTL)
	}
	// auth_time must survive, otherwise renewal would keep resetting the cap and
	// the session could be extended forever.
	authTime, ok := claimTime(claims, "auth_time")
	if !ok || authTime.Unix() != issued.Unix() {
		t.Fatalf("auth_time = %v, want the original %v", authTime, issued)
	}
	if !cookie.HttpOnly {
		t.Fatal("renewed cookie is not HttpOnly")
	}
}

func TestRenewStopsAtMaxLifetime(t *testing.T) {
	now := time.Now()
	authTime := now.Add(-(MaxLifetime + time.Hour))
	issued := now.Add(-(renewAfter + time.Hour))
	if _, renewed := renewedCookie(t, claimsFor(t, authTime, issued, TTL, false)); renewed {
		t.Fatal("renewed a session past MaxLifetime")
	}
}

func TestRenewTreatsLegacyTokenIatAsAuthTime(t *testing.T) {
	now := time.Now()
	issued := now.Add(-(renewAfter + time.Hour))
	cookie, renewed := renewedCookie(t, claimsFor(t, time.Time{}, issued, TTL, true))
	if !renewed {
		t.Fatal("did not renew a token minted before auth_time existed")
	}
	claims := parse(t, cookie.Value)
	if authTime, ok := claimTime(claims, "auth_time"); !ok || authTime.Unix() != issued.Unix() {
		t.Fatalf("auth_time = %v, want the legacy iat %v", authTime, issued)
	}
}

func TestRenewSkipsTokenWithoutSubject(t *testing.T) {
	now := time.Now()
	issued := now.Add(-(renewAfter + time.Hour))
	claims := claimsFor(t, issued, issued, TTL, false)
	delete(claims, "sub")
	if _, renewed := renewedCookie(t, claims); renewed {
		t.Fatal("renewed a token with no subject")
	}
}

func TestIssueMarksCookieSecureBehindTLSIngress(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")

	if err := Issue(rec, req, testSecret, "user-1", "jaennil", time.Now()); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	cookie := rec.Result().Cookies()[0]
	if !cookie.Secure {
		t.Fatal("cookie is not Secure behind an https ingress")
	}
	if cookie.MaxAge != int(TTL/time.Second) {
		t.Fatalf("MaxAge = %d, want %d", cookie.MaxAge, int(TTL/time.Second))
	}
}

func TestClearExpiresCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	Clear(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	cookie := rec.Result().Cookies()[0]
	if cookie.MaxAge >= 0 {
		t.Fatalf("MaxAge = %d, want a negative value that expires the cookie", cookie.MaxAge)
	}
}
