package grpc_logger_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zauberhaus/logger/pkg/grpc_logger"
	"github.com/zauberhaus/logger/pkg/mock"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// containsStr returns a gomock matcher that checks whether a string argument contains sub.
func containsStr(sub string) gomock.Matcher { return substrMatcher{sub: sub} }

type substrMatcher struct{ sub string }

func (c substrMatcher) Matches(x any) bool {
	s, ok := x.(string)
	return ok && strings.Contains(s, c.sub)
}
func (c substrMatcher) String() string { return fmt.Sprintf("contains %q", c.sub) }

// --- fake stream helpers ---

type fakeClientStream struct {
	sendErr error
	recvErr error
}

func (f *fakeClientStream) Header() (metadata.MD, error) { return nil, nil }
func (f *fakeClientStream) Trailer() metadata.MD         { return nil }
func (f *fakeClientStream) CloseSend() error             { return nil }
func (f *fakeClientStream) Context() context.Context     { return context.Background() }
func (f *fakeClientStream) SendMsg(_ any) error          { return f.sendErr }
func (f *fakeClientStream) RecvMsg(_ any) error          { return f.recvErr }

type fakeServerStream struct {
	sendErr error
	recvErr error
}

func (f *fakeServerStream) SetHeader(_ metadata.MD) error  { return nil }
func (f *fakeServerStream) SendHeader(_ metadata.MD) error { return nil }
func (f *fakeServerStream) SetTrailer(_ metadata.MD)       {}
func (f *fakeServerStream) Context() context.Context       { return context.Background() }
func (f *fakeServerStream) SendMsg(_ any) error            { return f.sendErr }
func (f *fakeServerStream) RecvMsg(_ any) error            { return f.recvErr }

// --- UnaryClientInterceptor ---

func TestUnaryClientInterceptor_DebugDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(false)

	called := false
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		called = true
		return nil
	}

	interceptor := grpc_logger.UnaryClientInterceptor(m)
	err := interceptor(context.Background(), "/svc/Method", "req", "reply", nil, invoker)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestUnaryClientInterceptor_DebugEnabled_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(true)
	m.EXPECT().Debugf(containsStr("[GRPC CLIENT]"), gomock.Any(), gomock.Any())
	m.EXPECT().Debugf(containsStr("[GRPC CLIENT DONE]"), gomock.Any(), gomock.Any(), gomock.Any())

	interceptor := grpc_logger.UnaryClientInterceptor(m)
	err := interceptor(context.Background(), "/svc/Method", "req", "reply", nil,
		func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			return nil
		},
	)
	require.NoError(t, err)
}

func TestUnaryClientInterceptor_DebugEnabled_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	boom := errors.New("boom")
	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(true)
	m.EXPECT().Debugf(containsStr("[GRPC CLIENT]"), gomock.Any(), gomock.Any())
	m.EXPECT().Errorf(containsStr("[GRPC CLIENT ERROR]"), gomock.Any(), gomock.Any(), gomock.Any())

	interceptor := grpc_logger.UnaryClientInterceptor(m)
	err := interceptor(context.Background(), "/svc/Method", "req", "reply", nil,
		func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			return boom
		},
	)
	assert.ErrorIs(t, err, boom)
}

// --- UnaryServerInterceptor ---

func TestUnaryServerInterceptor_DebugDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(false)

	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "resp", nil
	}

	interceptor := grpc_logger.UnaryServerInterceptor(m)
	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, handler)
	require.NoError(t, err)
	assert.Equal(t, "resp", resp)
	assert.True(t, called)
}

func TestUnaryServerInterceptor_DebugEnabled_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(true)
	m.EXPECT().Debugf(containsStr("[GRPC SERVER]"), gomock.Any(), gomock.Any())
	m.EXPECT().Debugf(containsStr("[GRPC SERVER DONE]"), gomock.Any(), gomock.Any(), gomock.Any())

	interceptor := grpc_logger.UnaryServerInterceptor(m)
	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		func(_ context.Context, _ any) (any, error) { return "resp", nil },
	)
	require.NoError(t, err)
	assert.Equal(t, "resp", resp)
}

func TestUnaryServerInterceptor_DebugEnabled_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	boom := errors.New("boom")
	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(true)
	m.EXPECT().Debugf(containsStr("[GRPC SERVER]"), gomock.Any(), gomock.Any())
	m.EXPECT().Errorf(containsStr("[GRPC SERVER ERROR]"), gomock.Any(), gomock.Any(), gomock.Any())

	interceptor := grpc_logger.UnaryServerInterceptor(m)
	_, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		func(_ context.Context, _ any) (any, error) { return nil, boom },
	)
	assert.ErrorIs(t, err, boom)
}

// --- StreamClientInterceptor ---

func TestStreamClientInterceptor_DebugDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(false)

	base := &fakeClientStream{}
	interceptor := grpc_logger.StreamClientInterceptor(m)
	stream, err := interceptor(context.Background(), &grpc.StreamDesc{}, nil, "/svc/Stream",
		func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return base, nil
		},
	)
	require.NoError(t, err)
	assert.Equal(t, base, stream)
}

func TestStreamClientInterceptor_DebugEnabled_SendRecv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(true)
	m.EXPECT().Debugf(containsStr("[GRPC CLIENT STREAM]"), gomock.Any())
	m.EXPECT().Debugf(containsStr("[GRPC CLIENT STREAM OPENED]"), gomock.Any(), gomock.Any())
	m.EXPECT().Debugf(containsStr("[GRPC CLIENT STREAM SEND]"), gomock.Any(), gomock.Any())
	m.EXPECT().Debugf(containsStr("[GRPC CLIENT STREAM RECV]"), gomock.Any(), gomock.Any())

	interceptor := grpc_logger.StreamClientInterceptor(m)
	stream, err := interceptor(context.Background(), &grpc.StreamDesc{}, nil, "/svc/Stream",
		func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &fakeClientStream{}, nil
		},
	)
	require.NoError(t, err)

	require.NoError(t, stream.SendMsg("hello"))
	require.NoError(t, stream.RecvMsg(nil))
}

func TestStreamClientInterceptor_DebugEnabled_RecvEOF(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(true)
	m.EXPECT().Debugf(containsStr("[GRPC CLIENT STREAM]"), gomock.Any())
	m.EXPECT().Debugf(containsStr("[GRPC CLIENT STREAM OPENED]"), gomock.Any(), gomock.Any())

	interceptor := grpc_logger.StreamClientInterceptor(m)
	stream, err := interceptor(context.Background(), &grpc.StreamDesc{}, nil, "/svc/Stream",
		func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &fakeClientStream{recvErr: io.EOF}, nil
		},
	)
	require.NoError(t, err)

	// EOF must not produce a debug log (stream ended normally)
	assert.ErrorIs(t, stream.RecvMsg(nil), io.EOF)
}

// --- StreamServerInterceptor ---

func TestStreamServerInterceptor_DebugDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(false)

	called := false
	interceptor := grpc_logger.StreamServerInterceptor(m)
	err := interceptor(nil, &fakeServerStream{}, &grpc.StreamServerInfo{FullMethod: "/svc/Stream"},
		func(_ any, _ grpc.ServerStream) error {
			called = true
			return nil
		},
	)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestStreamServerInterceptor_DebugEnabled_SendRecv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(true)
	m.EXPECT().Debugf(containsStr("[GRPC SERVER STREAM]"), gomock.Any())
	m.EXPECT().Debugf(containsStr("[GRPC SERVER STREAM SEND]"), gomock.Any(), gomock.Any())
	m.EXPECT().Debugf(containsStr("[GRPC SERVER STREAM RECV]"), gomock.Any(), gomock.Any())
	m.EXPECT().Debugf(containsStr("[GRPC SERVER STREAM DONE]"), gomock.Any(), gomock.Any())

	interceptor := grpc_logger.StreamServerInterceptor(m)
	err := interceptor(nil, &fakeServerStream{}, &grpc.StreamServerInfo{FullMethod: "/svc/Stream"},
		func(_ any, ss grpc.ServerStream) error {
			_ = ss.SendMsg("hello")
			_ = ss.RecvMsg(nil)
			return nil
		},
	)
	require.NoError(t, err)
}

func TestStreamServerInterceptor_DebugEnabled_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	boom := errors.New("boom")
	m := mock.NewMockLogger(ctrl)
	m.EXPECT().IsDebugEnabled().Return(true)
	m.EXPECT().Debugf(containsStr("[GRPC SERVER STREAM]"), gomock.Any())
	m.EXPECT().Errorf(containsStr("[GRPC SERVER STREAM ERROR]"), gomock.Any(), gomock.Any(), gomock.Any())

	interceptor := grpc_logger.StreamServerInterceptor(m)
	err := interceptor(nil, &fakeServerStream{}, &grpc.StreamServerInfo{FullMethod: "/svc/Stream"},
		func(_ any, _ grpc.ServerStream) error { return boom },
	)
	assert.ErrorIs(t, err, boom)
}
