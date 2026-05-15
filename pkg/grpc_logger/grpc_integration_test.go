package grpc_logger_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zauberhaus/logger/pkg/grpc_logger"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/memory"
	"github.com/zauberhaus/logger/pkg/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/test/bufconn"
)

func init() {
	// Replace the default proto codec with JSON so the integration test
	// needs no generated proto code.
	encoding.RegisterCodec(jsonCodec{})
}

type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
func (jsonCodec) Name() string                       { return "proto" }

// echoMsg is the request/response type for the test service.
type echoMsg struct {
	Msg string `json:"msg"`
}

// echoSvcServer is the interface gRPC checks against the registered impl.
type echoSvcServer interface {
	unary(ctx context.Context, req *echoMsg) (*echoMsg, error)
	stream(req *echoMsg, ss grpc.ServerStream) error
}

type echoImpl struct{}

func (*echoImpl) unary(_ context.Context, req *echoMsg) (*echoMsg, error) {
	return &echoMsg{Msg: "echo:" + req.Msg}, nil
}

func (*echoImpl) stream(req *echoMsg, ss grpc.ServerStream) error {
	for i := range 3 {
		if err := ss.SendMsg(&echoMsg{Msg: fmt.Sprintf("[%d] %s", i, req.Msg)}); err != nil {
			return err
		}
	}
	return nil
}

var echoSvcDesc = grpc.ServiceDesc{
	ServiceName: "test.EchoSvc",
	HandlerType: (*echoSvcServer)(nil),
	Methods: []grpc.MethodDesc{{
		MethodName: "Unary",
		Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
			req := new(echoMsg)
			if err := dec(req); err != nil {
				return nil, err
			}
			if interceptor == nil {
				return srv.(echoSvcServer).unary(ctx, req)
			}
			info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/test.EchoSvc/Unary"}
			return interceptor(ctx, req, info, func(ctx context.Context, req any) (any, error) {
				return srv.(echoSvcServer).unary(ctx, req.(*echoMsg))
			})
		},
	}},
	Streams: []grpc.StreamDesc{{
		StreamName:    "Stream",
		ServerStreams: true,
		Handler: func(srv any, ss grpc.ServerStream) error {
			req := new(echoMsg)
			if err := ss.RecvMsg(req); err != nil {
				return err
			}
			return srv.(echoSvcServer).stream(req, ss)
		},
	}},
}

func startEchoServer(t *testing.T, l logger.Logger) *bufconn.Listener {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(grpc_logger.UnaryServerInterceptor(l)),
		grpc.StreamInterceptor(grpc_logger.StreamServerInterceptor(l)),
	)
	srv.RegisterService(&echoSvcDesc, &echoImpl{})
	go srv.Serve(lis) //nolint:errcheck
	t.Cleanup(func() {
		srv.GracefulStop()
		lis.Close()
	})
	return lis
}

func dialEchoClient(t *testing.T, lis *bufconn.Listener, l logger.Logger) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(grpc_logger.UnaryClientInterceptor(l)),
		grpc.WithStreamInterceptor(grpc_logger.StreamClientInterceptor(l)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func debugLogger() *memory.MemoryLogger {
	return memory.NewLogger(zap.WithLevel(logger.DebugLevel), zap.WithoutCaller())
}

func infoLogger() *memory.MemoryLogger {
	return memory.NewLogger(zap.WithLevel(logger.InfoLevel), zap.WithoutCaller())
}

// --- unary ---

func TestIntegration_Unary_DebugEnabled(t *testing.T) {
	mem := debugLogger()
	lis := startEchoServer(t, mem)
	conn := dialEchoClient(t, lis, mem)

	resp := new(echoMsg)
	err := conn.Invoke(context.Background(), "/test.EchoSvc/Unary", &echoMsg{Msg: "hello"}, resp)
	require.NoError(t, err)
	assert.Equal(t, "echo:hello", resp.Msg)

	// Client logs request + done; server logs request + done — 4 lines total.
	logs := string(mem.Bytes())
	assert.Contains(t, logs, "[GRPC CLIENT]")
	assert.Contains(t, logs, "[GRPC CLIENT DONE]")
	assert.Contains(t, logs, "[GRPC SERVER]")
	assert.Contains(t, logs, "[GRPC SERVER DONE]")
	assert.Contains(t, logs, "/test.EchoSvc/Unary")
}

func TestIntegration_Unary_DebugDisabled(t *testing.T) {
	mem := infoLogger()
	lis := startEchoServer(t, mem)
	conn := dialEchoClient(t, lis, mem)

	resp := new(echoMsg)
	err := conn.Invoke(context.Background(), "/test.EchoSvc/Unary", &echoMsg{Msg: "hello"}, resp)
	require.NoError(t, err)
	assert.Equal(t, "echo:hello", resp.Msg)

	assert.Equal(t, 0, mem.Len())
}

// --- server-streaming ---

func TestIntegration_Stream_DebugEnabled(t *testing.T) {
	mem := debugLogger()
	lis := startEchoServer(t, mem)
	conn := dialEchoClient(t, lis, mem)

	stream, err := conn.NewStream(context.Background(),
		&grpc.StreamDesc{ServerStreams: true}, "/test.EchoSvc/Stream")
	require.NoError(t, err)
	require.NoError(t, stream.SendMsg(&echoMsg{Msg: "hello"}))
	require.NoError(t, stream.CloseSend())

	var msgs []echoMsg
	for {
		m := new(echoMsg)
		if err := stream.RecvMsg(m); err == io.EOF {
			break
		} else {
			require.NoError(t, err)
		}
		msgs = append(msgs, *m)
	}
	require.Len(t, msgs, 3)
	assert.Equal(t, "[0] hello", msgs[0].Msg)

	// Client: Opening, Opened, Send×1, Recv×3
	// Server: Started, Recv×1 (initial req), Send×3, Done
	logs := string(mem.Bytes())
	assert.Contains(t, logs, "[GRPC CLIENT STREAM]")
	assert.Contains(t, logs, "[GRPC CLIENT STREAM OPENED]")
	assert.Contains(t, logs, "[GRPC CLIENT STREAM SEND]")
	assert.Contains(t, logs, "[GRPC CLIENT STREAM RECV]")
	assert.Contains(t, logs, "[GRPC SERVER STREAM]")
	assert.Contains(t, logs, "[GRPC SERVER STREAM RECV]")
	assert.Contains(t, logs, "[GRPC SERVER STREAM SEND]")
	assert.Contains(t, logs, "[GRPC SERVER STREAM DONE]")
	assert.Contains(t, logs, "/test.EchoSvc/Stream")
}

func TestIntegration_Stream_DebugDisabled(t *testing.T) {
	mem := infoLogger()
	lis := startEchoServer(t, mem)
	conn := dialEchoClient(t, lis, mem)

	stream, err := conn.NewStream(context.Background(),
		&grpc.StreamDesc{ServerStreams: true}, "/test.EchoSvc/Stream")
	require.NoError(t, err)
	require.NoError(t, stream.SendMsg(&echoMsg{Msg: "hello"}))
	require.NoError(t, stream.CloseSend())

	for {
		if err := stream.RecvMsg(new(echoMsg)); err == io.EOF {
			break
		} else {
			require.NoError(t, err)
		}
	}

	assert.Equal(t, 0, mem.Len())
}
