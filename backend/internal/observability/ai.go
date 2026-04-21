package observability

import "errors"

var (
	ErrAIUnavailable = errors.New("ai unavailable")
	ErrAIUpstream    = errors.New("ai upstream error")
	ErrAIBadResponse = errors.New("ai bad response")
)

func AIStatusFromError(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, ErrAIUnavailable):
		return "unavailable"
	case errors.Is(err, ErrAIUpstream):
		return "upstream_error"
	case errors.Is(err, ErrAIBadResponse):
		return "bad_response"
	default:
		return "internal_error"
	}
}
