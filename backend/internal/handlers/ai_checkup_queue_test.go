package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestShutdownAwareKeepsRealFailuresAtError(t *testing.T) {
	var buffer bytes.Buffer
	handler := &AIHandler{logger: zerolog.New(&buffer)}

	handler.shutdownAware(context.Canceled).Err(context.Canceled).Msg("claim checkup job")
	handler.shutdownAware(fmt.Errorf("wrapped: %w", context.Canceled)).Msg("retry checkup job")
	if strings.Contains(buffer.String(), `"level":"error"`) {
		t.Errorf("a shutdown was logged as an error: %s", buffer.String())
	}

	buffer.Reset()
	handler.shutdownAware(errors.New("connection refused")).Msg("claim checkup job")
	if !strings.Contains(buffer.String(), `"level":"error"`) {
		t.Errorf("a real failure lost its level: %s", buffer.String())
	}
}
