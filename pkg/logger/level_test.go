package logger_test

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger/pkg/logger"
)

func Ptr[T any](v T) *T {
	return &v
}

func TestLevel_Parse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		txt     string
		want    logger.Level
		wantErr bool
	}{
		{
			name: "debug",
			txt:  "debug",
			want: logger.DebugLevel,
		},
		{
			name: "info",
			txt:  "info",
			want: logger.InfoLevel,
		},
		{
			name:    "empty uses info",
			txt:     "",
			want:    logger.InfoLevel,
			wantErr: false,
		},
		{
			name: "warn",
			txt:  "warn",
			want: logger.WarnLevel,
		},
		{
			name: "error",
			txt:  "error",
			want: logger.ErrorLevel,
		},
		{
			name: "panic",
			txt:  "panic",
			want: logger.PanicLevel,
		},
		{
			name: "fatal",
			txt:  "fatal",
			want: logger.FatalLevel,
		},
		{
			name:    "unknown",
			txt:     "unknown",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var l logger.Level
			got, err := l.Parse(tt.txt)
			if (err != nil) != tt.wantErr {
				t.Errorf("Level.Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Level.Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLevel_MarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		l       logger.Level
		want    []byte
		wantErr bool
	}{
		{
			name: "debug",
			l:    logger.DebugLevel,
			want: []byte("debug"),
		},
		{
			name: "info",
			l:    logger.InfoLevel,
			want: []byte("info"),
		},
		{
			name: "warn",
			l:    logger.WarnLevel,
			want: []byte("warn"),
		},
		{
			name: "error",
			l:    logger.ErrorLevel,
			want: []byte("error"),
		},
		{
			name: "panic",
			l:    logger.PanicLevel,
			want: []byte("panic"),
		},
		{
			name: "fatal",
			l:    logger.FatalLevel,
			want: []byte("fatal"),
		},
		{
			name: "unknown",
			l:    logger.Level(99),
			want: []byte("Level(99)"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.l.MarshalText()
			if (err != nil) != tt.wantErr {
				t.Errorf("Level.MarshalText() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLevel_UnmarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		text    []byte
		want    logger.Level
		wantErr bool
	}{
		{
			name: "debug",
			text: []byte("debug"),
			want: logger.DebugLevel,
		},
		{
			name: "info",
			text: []byte("info"),
			want: logger.InfoLevel,
		},
		{
			name: "warn",
			text: []byte("warn"),
			want: logger.WarnLevel,
		},
		{
			name: "error",
			text: []byte("error"),
			want: logger.ErrorLevel,
		},
		{
			name: "panic",
			text: []byte("panic"),
			want: logger.PanicLevel,
		},
		{
			name: "fatal",
			text: []byte("fatal"),
			want: logger.FatalLevel,
		},
		{
			name:    "unknown",
			text:    []byte("unknown"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var l logger.Level
			err := l.UnmarshalText(tt.text)
			if (err != nil) != tt.wantErr {
				t.Errorf("Level.UnmarshalText() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				assert.Equal(t, tt.want, l)
			}
		})
	}
}

func TestLevel_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		l    logger.Level
		want string
	}{
		{
			name: "debug",
			l:    logger.DebugLevel,
			want: "debug",
		},
		{
			name: "info",
			l:    logger.InfoLevel,
			want: "info",
		},
		{
			name: "warn",
			l:    logger.WarnLevel,
			want: "warn",
		},
		{
			name: "error",
			l:    logger.ErrorLevel,
			want: "error",
		},
		{
			name: "panic",
			l:    logger.PanicLevel,
			want: "panic",
		},
		{
			name: "fatal",
			l:    logger.FatalLevel,
			want: "fatal",
		},
		{
			name: "unknown",
			l:    logger.Level(99),
			want: "Level(99)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.l.String(); got != tt.want {
				t.Errorf("Level.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLevel_All(t *testing.T) {
	t.Parallel()

	l := logger.DebugLevel
	all := l.All()

	assert.Len(t, all, 6)
	assert.Equal(t, "debug", all[logger.DebugLevel])
	assert.Equal(t, "info", all[logger.InfoLevel])
	assert.Equal(t, "warn", all[logger.WarnLevel])
	assert.Equal(t, "error", all[logger.ErrorLevel])
	assert.Equal(t, "panic", all[logger.PanicLevel])
	assert.Equal(t, "fatal", all[logger.FatalLevel])
}

func TestLevel_Names(t *testing.T) {
	t.Parallel()

	l := logger.DebugLevel
	names := l.Names()

	assert.Equal(t, []string{"debug", "info", "warn", "error", "panic", "fatal"}, names)
}

func TestLevel_PflagValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    logger.Level
		wantErr bool
	}{
		{name: "debug", value: "debug", want: logger.DebugLevel},
		{name: "info", value: "info", want: logger.InfoLevel},
		{name: "warn", value: "warn", want: logger.WarnLevel},
		{name: "error", value: "error", want: logger.ErrorLevel},
		{name: "panic", value: "panic", want: logger.PanicLevel},
		{name: "fatal", value: "fatal", want: logger.FatalLevel},
		{name: "invalid", value: "invalid", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var level logger.Level
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			fs.Var(&level, "log-level", "log level")

			err := fs.Parse([]string{"--log-level", tt.value})
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, level)
			}
		})
	}
}

func TestLevel_Type(t *testing.T) {
	t.Parallel()

	var l logger.Level
	assert.Equal(t, "level", l.Type())
}
