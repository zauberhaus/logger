package logger_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger"
	"github.com/zauberhaus/logger/pkg/memory"
)

func TestLogger(t *testing.T) {
	def := logger.GetLogger(context.TODO())
	assert.NotNil(t, def)

	l := memory.NewLogger()
	logger.SetLogger(l)
	l2 := logger.GetLogger(context.TODO())
	assert.Equal(t, l, l2)

	l3 := memory.NewLogger()
	ctx := logger.AddLogger(context.TODO(), l3)
	l4 := logger.GetLogger(ctx)
	assert.Equal(t, l3, l4)
}

func TestGet(t *testing.T) {
	def := memory.NewLogger()
	logger.SetLogger(def)

	assert.Equal(t, def, logger.Get(nil)) //nolint:staticcheck // nil context is handled
	assert.Equal(t, def, logger.Get(context.TODO()))
	assert.Equal(t, def, logger.Get(context.WithValue(context.TODO(), logger.LoggerKey, "not a logger")))

	l := memory.NewLogger()
	assert.Equal(t, l, logger.Get(logger.AddLogger(context.TODO(), l)))
}
