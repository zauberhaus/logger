package cloudwatch

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/zap/zapcore"

	zaplogger "github.com/zauberhaus/logger/pkg/zap"
)

// Option configures a Sink.
type Option func(*Sink)

// WithClient sets the CloudWatch Logs client, e.g. a configured
// *cloudwatchlogs.Client or a fake in tests. It takes precedence over
// WithAWSConfig.
func WithClient(client API) Option {
	return func(w *Sink) { w.client = client }
}

// WithAWSConfig sets the AWS configuration (region, credentials, ...) used to
// create the CloudWatch Logs client. Defaults to config.LoadDefaultConfig.
func WithAWSConfig(cfg aws.Config) Option {
	return func(w *Sink) { w.awsConfig = &cfg }
}

// WithCreateLogGroup creates the log group if it does not exist. This needs
// the logs:CreateLogGroup permission.
func WithCreateLogGroup() Option {
	return func(w *Sink) { w.createGroup = true }
}

// WithExistingLogStream writes to a log stream that already exists, so only
// the logs:PutLogEvents permission is needed. Unless WithCreateLogGroup is
// also set, NewSink then makes no AWS call, so a missing stream or permission
// is reported on the first delivery. A deleted stream is not recreated. By default the stream is created when
// missing, which needs logs:CreateLogStream.
func WithExistingLogStream() Option {
	return func(w *Sink) { w.existingStream = true }
}

// WithBatchSize sets the maximum number of events per PutLogEvents request.
// Defaults to 1000; values above the CloudWatch limit of 10000 are capped.
func WithBatchSize(n int) Option {
	return func(w *Sink) { w.batchSize = n }
}

// WithFlushInterval sets how often buffered events are flushed. Defaults to 5s.
func WithFlushInterval(d time.Duration) Option {
	return func(w *Sink) { w.flushInterval = d }
}

// WithQueueSize sets the capacity of the input queue in lines. When the queue
// is full, Write drops the line and reports ErrQueueFull. Defaults to 10000.
func WithQueueSize(n int) Option {
	return func(w *Sink) { w.queueSize = n }
}

// WithTimeout sets the timeout for each AWS request. Defaults to 10s.
func WithTimeout(d time.Duration) Option {
	return func(w *Sink) { w.timeout = d }
}

// WithErrorHandler registers a callback invoked on delivery errors, rejected
// events, dropped lines and writes after Close. When set, Write and Sync
// swallow these errors instead of returning them. Errors from background
// deliveries are only visible through this handler. The callback may be
// called concurrently.
func WithErrorHandler(fn func(error)) Option {
	return func(w *Sink) { w.onError = fn }
}

// WithCloudWatch returns a zap.Option that forwards logs to a CloudWatch Logs
// log stream. Pair with zap.WithOutput(zap.JSONOutput) to get structured
// events that CloudWatch Logs Insights can query.
func WithCloudWatch(group, stream string, opts ...Option) (zaplogger.Option, error) {
	w, err := NewSink(group, stream, opts...)
	if err != nil {
		return nil, err
	}
	return zaplogger.WithWriteSyncer(zapcore.AddSync(w)), nil
}
