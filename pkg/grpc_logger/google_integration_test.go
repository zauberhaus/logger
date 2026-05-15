// cspell:ignore Warningf Warningln Errorln Infoln
package grpc_logger_test

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zauberhaus/logger/pkg/grpc_logger"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/memory"
	"github.com/zauberhaus/logger/pkg/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

// setFrameworkLogger registers l as gRPC's global framework logger and
// restores a no-op logger when the test ends.
func setFrameworkLogger(t *testing.T, l logger.Logger) {
	t.Helper()
	grpclog.SetLoggerV2(grpc_logger.NewLogger(l))
	t.Cleanup(func() {
		grpclog.SetLoggerV2(grpclog.NewLoggerV2(io.Discard, io.Discard, io.Discard))
	})
}

func TestIntegration_GrpcLogger_Info(t *testing.T) {
	mem := memory.NewLogger(zap.WithLevel(logger.InfoLevel), zap.WithoutCaller())
	setFrameworkLogger(t, mem)

	grpclog.Info("plain info")
	grpclog.Infoln("line info")
	grpclog.Infof("fmt %s", "info")

	logs := string(mem.Bytes())
	assert.Contains(t, logs, "plain info")
	assert.Contains(t, logs, "line info")
	assert.Contains(t, logs, "fmt info")
}

func TestIntegration_GrpcLogger_Warning(t *testing.T) {
	mem := memory.NewLogger(zap.WithLevel(logger.InfoLevel), zap.WithoutCaller())
	setFrameworkLogger(t, mem)

	grpclog.Warning("plain warn")
	grpclog.Warningln("line warn")
	grpclog.Warningf("fmt %s", "warn")

	logs := string(mem.Bytes())
	assert.Contains(t, logs, "plain warn")
	assert.Contains(t, logs, "line warn")
	assert.Contains(t, logs, "fmt warn")
}

func TestIntegration_GrpcLogger_Error(t *testing.T) {
	mem := memory.NewLogger(zap.WithLevel(logger.InfoLevel), zap.WithoutCaller())
	setFrameworkLogger(t, mem)

	grpclog.Error("plain error")
	grpclog.Errorln("line error")
	grpclog.Errorf("fmt %s", "error")

	logs := string(mem.Bytes())
	assert.Contains(t, logs, "plain error")
	assert.Contains(t, logs, "line error")
	assert.Contains(t, logs, "fmt error")
}

func TestIntegration_GrpcLogger_V(t *testing.T) {
	t.Run("debug enabled", func(t *testing.T) {
		mem := memory.NewLogger(zap.WithLevel(logger.DebugLevel), zap.WithoutCaller())
		setFrameworkLogger(t, mem)
		assert.True(t, grpclog.V(0))
		assert.True(t, grpclog.V(1))
	})

	t.Run("debug disabled", func(t *testing.T) {
		mem := memory.NewLogger(zap.WithLevel(logger.InfoLevel), zap.WithoutCaller())
		setFrameworkLogger(t, mem)
		assert.True(t, grpclog.V(0))
		assert.False(t, grpclog.V(1))
	})
}

func TestIntegration_GrpcLogger_WithRPC(t *testing.T) {
	mem := memory.NewLogger(zap.WithLevel(logger.InfoLevel), zap.WithoutCaller())
	setFrameworkLogger(t, mem)

	// Run a real RPC with the GrpcLogger active as the framework logger.
	// Any gRPC-internal warning/error logs will flow into mem; the call
	// succeeding confirms the logger does not break gRPC's operation.
	lis := startEchoServer(t, infoLogger())
	conn := dialEchoClient(t, lis, infoLogger())

	resp := new(echoMsg)
	err := conn.Invoke(context.Background(), "/test.EchoSvc/Unary", &echoMsg{Msg: "hi"}, resp)
	require.NoError(t, err)
	assert.Equal(t, "echo:hi", resp.Msg)

	// Emit a known message through the framework logger and confirm it
	// appears alongside any gRPC-generated output.
	grpclog.Info("rpc completed")
	assert.Contains(t, string(mem.Bytes()), "rpc completed")
}

func TestIntegration_GrpcLogger_ServerAndClient(t *testing.T) {
	mem := memory.NewLogger(zap.WithLevel(logger.InfoLevel), zap.WithoutCaller())
	setFrameworkLogger(t, mem)

	lis := startEchoServer(t, infoLogger())
	conn := dialEchoClient(t, lis, infoLogger())

	// --- unary ---
	grpclog.Infof("unary: sending request")
	resp := new(echoMsg)
	err := conn.Invoke(context.Background(), "/test.EchoSvc/Unary", &echoMsg{Msg: "hello"}, resp)
	require.NoError(t, err)
	assert.Equal(t, "echo:hello", resp.Msg)
	grpclog.Infof("unary: got response %q", resp.Msg)

	// --- server-streaming ---
	grpclog.Infof("stream: opening")
	stream, err := conn.NewStream(context.Background(),
		&grpc.StreamDesc{ServerStreams: true}, "/test.EchoSvc/Stream")
	require.NoError(t, err)
	require.NoError(t, stream.SendMsg(&echoMsg{Msg: "world"}))
	require.NoError(t, stream.CloseSend())

	var msgs []string
	for {
		m := new(echoMsg)
		if err := stream.RecvMsg(m); err == io.EOF {
			break
		} else {
			require.NoError(t, err)
		}
		msgs = append(msgs, m.Msg)
	}
	require.Len(t, msgs, 3)
	assert.Equal(t, "[0] world", msgs[0])
	grpclog.Infof("stream: received %d messages", len(msgs))

	logs := string(mem.Bytes())
	assert.Contains(t, logs, "unary: sending request")
	assert.Contains(t, logs, `unary: got response "echo:hello"`)
	assert.Contains(t, logs, "stream: opening")
	assert.Contains(t, logs, "stream: received 3 messages")
}
