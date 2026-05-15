// cspell:ignore Warningf Warningln Errorln Infoln
package grpc_logger

import (
	"fmt"

	"github.com/zauberhaus/logger/pkg/logger"
	"google.golang.org/grpc/grpclog"
)

type GrpcLogger struct {
	logger logger.Logger
}

func NewLogger(l logger.Logger) grpclog.LoggerV2 {
	return &GrpcLogger{logger: l}
}

func (g *GrpcLogger) Info(args ...any) {
	g.logger.Info(args...)
}

func (g *GrpcLogger) Infoln(args ...any) {
	g.logger.Info(fmt.Sprint(args...))
}

func (g *GrpcLogger) Infof(format string, args ...any) {
	g.logger.Infof(format, args...)
}

func (g *GrpcLogger) Warning(args ...any) {
	g.logger.Warn(args...)
}

func (g *GrpcLogger) Warningln(args ...any) {
	g.logger.Warn(fmt.Sprint(args...))
}

func (g *GrpcLogger) Warningf(format string, args ...any) {
	g.logger.Warnf(format, args...)
}

func (g *GrpcLogger) Error(args ...any) {
	g.logger.Error(args...)
}

func (g *GrpcLogger) Errorln(args ...any) {
	g.logger.Error(fmt.Sprint(args...))
}

func (g *GrpcLogger) Errorf(format string, args ...any) {
	g.logger.Errorf(format, args...)
}

func (g *GrpcLogger) Fatal(args ...any) {
	g.logger.Fatal(args...)
}

func (g *GrpcLogger) Fatalln(args ...any) {
	g.logger.Fatal(fmt.Sprint(args...))
}

func (g *GrpcLogger) Fatalf(format string, args ...any) {
	g.logger.Fatalf(format, args...)
}

// V returns true if the given verbosity level is enabled.
// Level 0 maps to info, higher levels require debug to be enabled.
func (g *GrpcLogger) V(l int) bool {
	if l == 0 {
		return g.logger.HasLevel(logger.InfoLevel)
	}
	return g.logger.IsDebugEnabled()
}
