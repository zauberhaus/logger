package cloudwatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/zauberhaus/logger/pkg/logger"
)

const (
	defaultBatchSize     = 1000
	defaultFlushInterval = 5 * time.Second
	defaultQueueSize     = 10000
	defaultTimeout       = 10 * time.Second

	maxBatchEvents = 10000
	maxBatchBytes  = 1048576
	eventOverhead  = 26
	maxEventBytes  = maxBatchBytes - eventOverhead
	maxBatchSpan   = 24 * time.Hour
)

var (
	ErrQueueFull = errors.New("cloudwatch: queue full, log line dropped")
	ErrClosed    = errors.New("cloudwatch: sink closed")
	ErrRejected  = errors.New("cloudwatch: log events rejected")
)

// API is the subset of the CloudWatch Logs client used by Sink.
type API interface {
	PutLogEvents(ctx context.Context, params *cloudwatchlogs.PutLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error)
	CreateLogStream(ctx context.Context, params *cloudwatchlogs.CreateLogStreamInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error)
	CreateLogGroup(ctx context.Context, params *cloudwatchlogs.CreateLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error)
}

type event struct {
	timestamp int64 // milliseconds since the Unix epoch
	message   string
}

// Sink ships logs to an AWS CloudWatch Logs log stream.
type Sink struct {
	group  string
	stream string

	client         API
	awsConfig      *aws.Config
	createGroup    bool
	existingStream bool
	timeout        time.Duration
	onError        func(error)

	batchSize     int
	flushInterval time.Duration
	queueSize     int

	mu     sync.RWMutex
	closed bool

	queue     chan event
	syncReq   chan chan error
	stop      chan struct{}
	done      chan struct{}
	closeErr  error
	closeOnce sync.Once

	// Owned by the run goroutine.
	events []event
	size   int
}

func NewSink(group, stream string, opts ...Option) (logger.Sink, error) {
	w := &Sink{
		group:         group,
		stream:        stream,
		timeout:       defaultTimeout,
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
		queueSize:     defaultQueueSize,
	}
	for _, opt := range opts {
		opt(w)
	}

	if w.group == "" || w.stream == "" {
		return nil, errors.New("cloudwatch: log group and stream are required")
	}
	if w.batchSize <= 0 || w.batchSize > maxBatchEvents {
		w.batchSize = maxBatchEvents
	}

	if w.client == nil {
		cfg, err := w.loadConfig()
		if err != nil {
			return nil, fmt.Errorf("cloudwatch: load AWS config: %w", err)
		}
		w.client = cloudwatchlogs.NewFromConfig(cfg)
	}

	if err := w.setup(); err != nil {
		return nil, err
	}

	w.queue = make(chan event, w.queueSize)
	w.syncReq = make(chan chan error)
	w.stop = make(chan struct{})
	w.done = make(chan struct{})
	go w.run()

	return w, nil
}

// Write enqueues a log line without waiting for delivery. Implements io.Writer
// and zapcore.WriteSyncer.
func (w *Sink) Write(p []byte) (int, error) {
	line := bytes.TrimRight(p, "\r\n")
	if len(line) == 0 {
		return len(p), nil
	}

	ev := event{
		timestamp: time.Now().UnixMilli(),
		message:   truncate(line, maxEventBytes),
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return 0, w.handleError(ErrClosed)
	}

	select {
	case w.queue <- ev:
		return len(p), nil
	default:
		return 0, w.handleError(ErrQueueFull)
	}
}

// Sync delivers all lines written before the call and waits for the result.
// Implements zapcore.WriteSyncer.
func (w *Sink) Sync() error {
	reply := make(chan error, 1)
	select {
	case w.syncReq <- reply:
		return <-reply
	case <-w.done:
		return nil
	}
}

// Close stops accepting lines, delivers everything still queued and stops
// the background worker.
func (w *Sink) Close() error {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		w.mu.Unlock()

		close(w.stop)
		<-w.done
	})
	return w.closeErr
}

func (w *Sink) loadConfig() (aws.Config, error) {
	if w.awsConfig != nil {
		return *w.awsConfig, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()
	return awsconfig.LoadDefaultConfig(ctx)
}

// setup creates the log group (if enabled) and the log stream. It doubles as
// a check of credentials and permissions.
func (w *Sink) setup() error {
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()

	if w.createGroup {
		_, err := w.client.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(w.group),
		})
		if err != nil && !isAlreadyExists(err) {
			return fmt.Errorf("cloudwatch: create log group %q: %w", w.group, err)
		}
	}

	if w.existingStream {
		return nil
	}
	if err := w.createStream(ctx); err != nil {
		return fmt.Errorf("cloudwatch: create log stream %q: %w", w.stream, err)
	}
	return nil
}

func (w *Sink) createStream(ctx context.Context) error {
	_, err := w.client.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(w.group),
		LogStreamName: aws.String(w.stream),
	})
	if err != nil && !isAlreadyExists(err) {
		return err
	}
	return nil
}

func (w *Sink) run() {
	defer close(w.done)

	t := time.NewTicker(w.flushInterval)
	defer t.Stop()

	for {
		select {
		case ev := <-w.queue:
			w.add(ev)
		case <-t.C:
			w.flush()
		case reply := <-w.syncReq:
			w.drain()
			reply <- w.flush()
		case <-w.stop:
			// Close holds no writers past this point, so the queue is final.
			w.drain()
			w.closeErr = w.flush()
			return
		}
	}
}

// add appends an event to the current batch, first flushing the batch if the
// event would exceed one of the PutLogEvents limits.
func (w *Sink) add(ev event) {
	size := len(ev.message) + eventOverhead
	if len(w.events) > 0 {
		span := time.Duration(ev.timestamp-w.events[0].timestamp) * time.Millisecond
		if len(w.events) >= w.batchSize || w.size+size > maxBatchBytes || span > maxBatchSpan {
			w.flush()
		}
	}

	w.events = append(w.events, ev)
	w.size += size

	if len(w.events) >= w.batchSize {
		w.flush()
	}
}

// drain moves every event currently in the queue into batches.
func (w *Sink) drain() {
	for n := len(w.queue); n > 0; n-- {
		w.add(<-w.queue)
	}
}

// flush sends the current batch. Must only be called from the run goroutine.
func (w *Sink) flush() error {
	if len(w.events) == 0 {
		return nil
	}
	events := w.events
	w.events = nil
	w.size = 0

	// Concurrent writers can enqueue slightly out of order, but CloudWatch
	// requires chronological order within a batch.
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].timestamp < events[j].timestamp
	})

	input := &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(w.group),
		LogStreamName: aws.String(w.stream),
		LogEvents:     make([]types.InputLogEvent, len(events)),
	}
	for i, ev := range events {
		input.LogEvents[i] = types.InputLogEvent{
			Timestamp: aws.Int64(ev.timestamp),
			Message:   aws.String(ev.message),
		}
	}

	out, err := w.put(input)
	if err != nil {
		return w.handleError(fmt.Errorf("cloudwatch: put log events: %w", err))
	}
	if err := rejected(out.RejectedLogEventsInfo, len(events)); err != nil {
		return w.handleError(err)
	}
	return nil
}

// put sends a batch, recreating the log stream once if it has been deleted
// (unless WithExistingLogStream is set).
func (w *Sink) put(input *cloudwatchlogs.PutLogEventsInput) (*cloudwatchlogs.PutLogEventsOutput, error) {
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()

	out, err := w.client.PutLogEvents(ctx, input)

	if _, ok := errors.AsType[*types.ResourceNotFoundException](err); ok && !w.existingStream {
		if createErr := w.createStream(ctx); createErr != nil {
			return nil, errors.Join(err, createErr)
		}
		out, err = w.client.PutLogEvents(ctx, input)
	}
	return out, err
}

func (w *Sink) handleError(err error) error {
	if w.onError != nil {
		w.onError(err)
		return nil
	}
	return err
}

// rejected describes the events CloudWatch rejected from a batch of n.
func rejected(info *types.RejectedLogEventsInfo, n int) error {
	if info == nil {
		return nil
	}

	var parts []string
	if i := info.TooOldLogEventEndIndex; i != nil {
		parts = append(parts, fmt.Sprintf("%d too old", *i+1))
	}
	if i := info.ExpiredLogEventEndIndex; i != nil {
		parts = append(parts, fmt.Sprintf("%d expired", *i+1))
	}
	if i := info.TooNewLogEventStartIndex; i != nil {
		parts = append(parts, fmt.Sprintf("%d too new", n-int(*i)))
	}
	if len(parts) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrRejected, strings.Join(parts, ", "))
}

func isAlreadyExists(err error) bool {
	_, ok := errors.AsType[*types.ResourceAlreadyExistsException](err)
	return ok
}

// truncate returns b as a valid UTF-8 string of at most max bytes. Invalid
// sequences are replaced first, then the string is cut at a rune boundary.
func truncate(b []byte, max int) string {
	s := strings.ToValidUTF8(string(b), "�")
	if len(s) <= max {
		return s
	}

	n := max
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
