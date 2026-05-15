package grpc_logger

import (
	"context"
	"io"
	"time"

	"github.com/zauberhaus/logger/pkg/logger"
	"google.golang.org/grpc"
)

// UnaryClientInterceptor returns a grpc.UnaryClientInterceptor that logs
// requests and responses at debug level.
func UnaryClientInterceptor(l logger.Logger) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if !l.IsDebugEnabled() {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		l.Debugf("[GRPC CLIENT] Method: %s | Request: %v", method, req)
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		if err != nil {
			l.Errorf("[GRPC CLIENT ERROR] Method: %s | Error: %v | Duration: %v", method, err, duration)
		} else {
			l.Debugf("[GRPC CLIENT DONE] Method: %s | Response: %v | Duration: %v", method, reply, duration)
		}

		return err
	}
}

// UnaryServerInterceptor returns a grpc.UnaryServerInterceptor that logs
// requests and responses at debug level.
func UnaryServerInterceptor(l logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !l.IsDebugEnabled() {
			return handler(ctx, req)
		}

		l.Debugf("[GRPC SERVER] Method: %s | Request: %v", info.FullMethod, req)
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		if err != nil {
			l.Errorf("[GRPC SERVER ERROR] Method: %s | Error: %v | Duration: %v", info.FullMethod, err, duration)
		} else {
			l.Debugf("[GRPC SERVER DONE] Method: %s | Response: %v | Duration: %v", info.FullMethod, resp, duration)
		}

		return resp, err
	}
}

// StreamClientInterceptor returns a grpc.StreamClientInterceptor that logs
// stream lifecycle and messages at debug level.
func StreamClientInterceptor(l logger.Logger) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if !l.IsDebugEnabled() {
			return streamer(ctx, desc, cc, method, opts...)
		}

		l.Debugf("[GRPC CLIENT STREAM] Method: %s | Opening", method)
		start := time.Now()
		stream, err := streamer(ctx, desc, cc, method, opts...)
		duration := time.Since(start)

		if err != nil {
			l.Errorf("[GRPC CLIENT STREAM ERROR] Method: %s | Error: %v | Duration: %v", method, err, duration)
			return nil, err
		}

		l.Debugf("[GRPC CLIENT STREAM OPENED] Method: %s | Duration: %v", method, duration)

		return &loggingClientStream{
			ClientStream: stream,
			method:       method,
			logger:       l,
		}, nil
	}
}

// StreamServerInterceptor returns a grpc.StreamServerInterceptor that logs
// stream lifecycle and messages at debug level.
func StreamServerInterceptor(l logger.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !l.IsDebugEnabled() {
			return handler(srv, ss)
		}

		l.Debugf("[GRPC SERVER STREAM] Method: %s | Started", info.FullMethod)
		start := time.Now()

		err := handler(srv, &loggingServerStream{
			ServerStream: ss,
			method:       info.FullMethod,
			logger:       l,
		})
		duration := time.Since(start)

		if err != nil {
			l.Errorf("[GRPC SERVER STREAM ERROR] Method: %s | Error: %v | Duration: %v", info.FullMethod, err, duration)
		} else {
			l.Debugf("[GRPC SERVER STREAM DONE] Method: %s | Duration: %v", info.FullMethod, duration)
		}

		return err
	}
}

type loggingClientStream struct {
	grpc.ClientStream
	method string
	logger logger.Logger
}

func (s *loggingClientStream) SendMsg(m any) error {
	s.logger.Debugf("[GRPC CLIENT STREAM SEND] Method: %s | Message: %v", s.method, m)
	return s.ClientStream.SendMsg(m)
}

func (s *loggingClientStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err == nil {
		s.logger.Debugf("[GRPC CLIENT STREAM RECV] Method: %s | Message: %v", s.method, m)
	} else if err != io.EOF {
		s.logger.Debugf("[GRPC CLIENT STREAM RECV ERROR] Method: %s | Error: %v", s.method, err)
	}
	return err
}

type loggingServerStream struct {
	grpc.ServerStream
	method string
	logger logger.Logger
}

func (s *loggingServerStream) SendMsg(m any) error {
	s.logger.Debugf("[GRPC SERVER STREAM SEND] Method: %s | Message: %v", s.method, m)
	return s.ServerStream.SendMsg(m)
}

func (s *loggingServerStream) RecvMsg(m any) error {
	err := s.ServerStream.RecvMsg(m)
	if err == nil {
		s.logger.Debugf("[GRPC SERVER STREAM RECV] Method: %s | Message: %v", s.method, m)
	} else if err != io.EOF {
		s.logger.Debugf("[GRPC SERVER STREAM RECV ERROR] Method: %s | Error: %v", s.method, err)
	}
	return err
}
