//go:generate go run go.uber.org/mock/mockgen@latest -package mock --typed --write_package_comment=false -destination=../mock/sentry_transport_mock.go -mock_names=Transport=MockSentryTransport github.com/getsentry/sentry-go Transport

// github.com/getsentry/sentry-go
// sentry.Client
package sentry

import (
	"fmt"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/random/pkg/stringer"
)

type Hub struct {
	hub *sentry.Hub
}

func New(hub *sentry.Hub) *Hub {
	return &Hub{
		hub: hub,
	}
}

func (h *Hub) Message(level logger.Level, args ...any) {
	if h != nil && h.hub != nil && h.hub.Client() != nil && h.hub.Scope() != nil {
		sl := Level(level)

		msg, err := h.toString(args)
		if err != nil {
			h.Capture(err)
			return
		}

		ev := h.hub.Client().EventFromMessage(msg, sl)
		h.hub.CaptureEvent(ev)

		if sl == sentry.LevelFatal {
			h.hub.Flush(time.Second * 5)
		}
	}
}

func (h *Hub) Capture(args ...any) {
	if h != nil && h.hub != nil && h.hub.Client() != nil && h.hub.Scope() != nil {
		changed := false

		for _, arg := range args {
			switch err := arg.(type) {
			case error:
				h.hub.CaptureException(err)
				changed = true
			}
		}

		if changed {
			h.hub.Flush(time.Second * 5)
		}
	}
}

func (h *Hub) toString(args []any) (string, error) {
	switch len(args) {
	case 0:
		return "", fmt.Errorf("message without args")
	case 1:
		return stringer.String(args[0])
	default:
		if s, ok := args[0].(string); ok {
			if strings.Contains(s, "%") {
				return fmt.Sprintf(s, args[1:]...), nil
			}
		}

		return stringer.String(args)
	}
}
