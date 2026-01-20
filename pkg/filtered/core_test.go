package filtered

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

func TestCheckedCore_Check(t *testing.T) {
	core := zaptest.NewLogger(t).Core()
	entry := zapcore.Entry{Message: "test message"}
	ce := &zapcore.CheckedEntry{}

	t.Run("no checks", func(t *testing.T) {
		c := &checked{core: core}
		assert.NotNil(t, c.Check(entry, ce))
	})

	t.Run("one check, pass", func(t *testing.T) {
		passCheck := func(ent zapcore.Entry, ce *zapcore.CheckedEntry) bool {
			return true
		}
		c := &checked{core: core, funcs: []checkFunc{passCheck}}
		assert.NotNil(t, c.Check(entry, ce))
	})

	t.Run("one check, fail", func(t *testing.T) {
		failCheck := func(ent zapcore.Entry, ce *zapcore.CheckedEntry) bool {
			return false
		}
		c := &checked{core: core, funcs: []checkFunc{failCheck}}
		assert.Nil(t, c.Check(entry, ce))
	})

	t.Run("multiple checks, all pass", func(t *testing.T) {
		passCheck1 := func(ent zapcore.Entry, ce *zapcore.CheckedEntry) bool { return true }
		passCheck2 := func(ent zapcore.Entry, ce *zapcore.CheckedEntry) bool { return true }
		c := &checked{core: core, funcs: []checkFunc{passCheck1, passCheck2}}
		assert.NotNil(t, c.Check(entry, ce))
	})

	t.Run("multiple checks, one fails", func(t *testing.T) {
		passCheck := func(ent zapcore.Entry, ce *zapcore.CheckedEntry) bool { return true }
		failCheck := func(ent zapcore.Entry, ce *zapcore.CheckedEntry) bool { return false }
		c := &checked{core: core, funcs: []checkFunc{passCheck, failCheck}}
		assert.Nil(t, c.Check(entry, ce))
	})
}

func TestCheckedCore_With(t *testing.T) {
	core := zaptest.NewLogger(t).Core()
	c := &checked{core: core}
	fields := []zapcore.Field{zapcore.Field{Key: "key", Type: zapcore.StringType, String: "value"}}
	withCore := c.With(fields)

	assert.NotNil(t, withCore)
	// We can't easily inspect the fields of the core, but we can check the type.
	_, ok := withCore.(*checked)
	assert.True(t, ok)
}

func TestCheckedCore_Write(t *testing.T) {
	core := zaptest.NewLogger(t).Core()
	c := &checked{core: core}
	err := c.Write(zapcore.Entry{}, nil)
	assert.NoError(t, err)
}

func TestCheckedCore_Enabled(t *testing.T) {
	core := zaptest.NewLogger(t).Core()
	c := &checked{core: core}
	assert.True(t, c.Enabled(zapcore.DebugLevel))
}

func TestCheckedCore_Sync(t *testing.T) {
	core := zaptest.NewLogger(t).Core()
	c := &checked{core: core}
	err := c.Sync()
	assert.NoError(t, err)
}

func TestCheckedCore_Level(t *testing.T) {
	core := zaptest.NewLogger(t).Core()
	c := &checked{core: core}
	assert.Equal(t, zapcore.DebugLevel, c.Level())
}
