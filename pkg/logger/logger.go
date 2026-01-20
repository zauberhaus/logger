//go:generate go run go.uber.org/mock/mockgen@latest --typed --write_package_comment=false -destination=logger_mock.go -source=./logger.go -package=logger

package logger

type (
	Logger interface {
		Debug(args ...any)
		Info(args ...any)
		Warn(args ...any)
		Error(args ...any)
		Panic(args ...any)
		Fatal(args ...any)

		Debugf(template string, args ...any)
		Infof(template string, args ...any)
		Warnf(template string, args ...any)
		Errorf(template string, args ...any)
		Panicf(template string, args ...any)
		Fatalf(template string, args ...any)

		With(args ...any) Logger

		AddSkip(steps int) Logger

		Level() Level
		SetLevel(level Level)
		HasLevel(level Level) bool
		EnableDebug()

		IsDebugEnabled() bool

		Sync() error
	}
)
