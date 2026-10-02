package zap

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

type syncer struct {
	err    error
	synced bool
}

func (s *syncer) Write(p []byte) (int, error) { return len(p), nil }

func (s *syncer) Sync() error {
	s.synced = true
	return s.err
}

func TestSplitCore_EnabledAndCheck(t *testing.T) {
	enc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{MessageKey: "msg"})
	c := newSplitCore(enc, &syncer{}, &syncer{}, zapcore.ErrorLevel)

	assert.True(t, c.Enabled(zapcore.DebugLevel))

	ent := zapcore.Entry{Level: zapcore.InfoLevel, Message: "x"}
	assert.NotNil(t, c.Check(ent, nil))
}

func TestSplitCore_Sync(t *testing.T) {
	lowErr := errors.New("low")
	highErr := errors.New("high")
	enc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{MessageKey: "msg"})

	tests := map[string]struct {
		low, high *syncer
		want      error
	}{
		"ok":         {low: &syncer{}, high: &syncer{}},
		"low fails":  {low: &syncer{err: lowErr}, high: &syncer{}, want: lowErr},
		"high fails": {low: &syncer{}, high: &syncer{err: highErr}, want: highErr},
		"both fail":  {low: &syncer{err: lowErr}, high: &syncer{err: highErr}, want: lowErr},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := newSplitCore(enc, tt.low, tt.high, zapcore.ErrorLevel)
			err := c.Sync()
			if tt.want == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tt.want)
			}
			// Both sides are synced even when one fails.
			assert.True(t, tt.low.synced)
			assert.True(t, tt.high.synced)
		})
	}
}
