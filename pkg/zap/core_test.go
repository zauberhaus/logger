package zap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type fakeCore struct {
	entries    []zapcore.Entry
	withFields [][]zapcore.Field
}

func (f *fakeCore) Enabled(zapcore.Level) bool { return true }

func (f *fakeCore) With(fields []zapcore.Field) zapcore.Core {
	f.withFields = append(f.withFields, fields)
	return f
}

func (f *fakeCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(ent, f)
}

func (f *fakeCore) Write(ent zapcore.Entry, _ []zapcore.Field) error {
	f.entries = append(f.entries, ent)
	return nil
}

func (f *fakeCore) Sync() error { return nil }

func TestNewLevelCore(t *testing.T) {
	inner := &fakeCore{}
	level := zap.NewAtomicLevelAt(zapcore.InfoLevel)

	c := newLevelCore(inner, level)

	assert.Same(t, inner, c.Core)
	assert.Equal(t, level, c.level)
}

func TestLevelCore_Enabled(t *testing.T) {
	inner := &fakeCore{}
	level := zap.NewAtomicLevelAt(zapcore.InfoLevel)
	c := newLevelCore(inner, level)

	assert.False(t, c.Enabled(zapcore.DebugLevel))
	assert.True(t, c.Enabled(zapcore.InfoLevel))
	assert.True(t, c.Enabled(zapcore.WarnLevel))

	level.SetLevel(zapcore.DebugLevel)
	assert.True(t, c.Enabled(zapcore.DebugLevel))
}

func TestLevelCore_Check(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		inner := &fakeCore{}
		level := zap.NewAtomicLevelAt(zapcore.InfoLevel)
		c := newLevelCore(inner, level)

		ent := zapcore.Entry{Level: zapcore.InfoLevel, Message: "hello"}
		ce := c.Check(ent, nil)
		if assert.NotNil(t, ce) {
			ce.Write()
		}

		if assert.Len(t, inner.entries, 1) {
			assert.Equal(t, "hello", inner.entries[0].Message)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		inner := &fakeCore{}
		level := zap.NewAtomicLevelAt(zapcore.InfoLevel)
		c := newLevelCore(inner, level)

		ent := zapcore.Entry{Level: zapcore.DebugLevel, Message: "should be dropped"}
		ce := c.Check(ent, nil)

		assert.Nil(t, ce)
		assert.Empty(t, inner.entries)
	})
}

func TestLevelCore_With(t *testing.T) {
	inner := &fakeCore{}
	level := zap.NewAtomicLevelAt(zapcore.InfoLevel)
	c := newLevelCore(inner, level)

	fields := []zapcore.Field{zap.String("key", "value")}
	clonedCore := c.With(fields)

	cloned, ok := clonedCore.(*levelCore)
	if !assert.True(t, ok) {
		return
	}

	// With() must share the parent's AtomicLevel: mutating one is visible via the other.
	assert.Equal(t, level, cloned.level)
	level.SetLevel(zapcore.DebugLevel)
	assert.True(t, cloned.Enabled(zapcore.DebugLevel))

	if assert.Len(t, inner.withFields, 1) {
		assert.Equal(t, fields, inner.withFields[0])
	}
}
