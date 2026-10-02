package cloudwatch_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/cloudwatch"
	zaplogger "github.com/zauberhaus/logger/pkg/zap"
)

// fakeAPI records CloudWatch Logs calls. putErr, if set, decides the error
// for the n-th PutLogEvents call (0-based).
type fakeAPI struct {
	mu      sync.Mutex
	groups  []string
	streams []string
	batches [][]types.InputLogEvent

	createGroupErr  error
	createStreamErr error
	putErr          func(n int) error
	putOut          *cloudwatchlogs.PutLogEventsOutput
	release         chan struct{} // if set, PutLogEvents blocks until closed
}

func (f *fakeAPI) CreateLogGroup(_ context.Context, in *cloudwatchlogs.CreateLogGroupInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groups = append(f.groups, aws.ToString(in.LogGroupName))
	return &cloudwatchlogs.CreateLogGroupOutput{}, f.createGroupErr
}

func (f *fakeAPI) CreateLogStream(_ context.Context, in *cloudwatchlogs.CreateLogStreamInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.streams = append(f.streams, aws.ToString(in.LogGroupName)+"/"+aws.ToString(in.LogStreamName))
	return &cloudwatchlogs.CreateLogStreamOutput{}, f.createStreamErr
}

func (f *fakeAPI) PutLogEvents(_ context.Context, in *cloudwatchlogs.PutLogEventsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error) {
	if f.release != nil {
		<-f.release
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	n := len(f.batches)
	f.batches = append(f.batches, in.LogEvents)
	if f.putErr != nil {
		if err := f.putErr(n); err != nil {
			return nil, err
		}
	}
	if f.putOut != nil {
		return f.putOut, nil
	}
	return &cloudwatchlogs.PutLogEventsOutput{}, nil
}

func (f *fakeAPI) messages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var msgs []string
	for _, b := range f.batches {
		for _, ev := range b {
			msgs = append(msgs, aws.ToString(ev.Message))
		}
	}
	return msgs
}

func (f *fakeAPI) batchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches)
}

func newSink(t *testing.T, api *fakeAPI, opts ...cloudwatch.Option) io.WriteCloser {
	t.Helper()

	s, err := cloudwatch.NewSink("group", "stream", append([]cloudwatch.Option{
		cloudwatch.WithClient(api),
		cloudwatch.WithFlushInterval(time.Hour),
	}, opts...)...)
	require.NoError(t, err)
	return s
}

func TestNewSink_RequiresGroupAndStream(t *testing.T) {
	_, err := cloudwatch.NewSink("", "stream", cloudwatch.WithClient(&fakeAPI{}))
	assert.ErrorContains(t, err, "log group and stream are required")

	_, err = cloudwatch.NewSink("group", "", cloudwatch.WithClient(&fakeAPI{}))
	assert.ErrorContains(t, err, "log group and stream are required")
}

func TestNewSink_CreatesStream(t *testing.T) {
	api := &fakeAPI{}
	s := newSink(t, api)
	require.NoError(t, s.Close())

	assert.Empty(t, api.groups)
	assert.Equal(t, []string{"group/stream"}, api.streams)
}

func TestNewSink_CreatesGroupWhenEnabled(t *testing.T) {
	api := &fakeAPI{}
	s := newSink(t, api, cloudwatch.WithCreateLogGroup())
	require.NoError(t, s.Close())

	assert.Equal(t, []string{"group"}, api.groups)
	assert.Equal(t, []string{"group/stream"}, api.streams)
}

func TestNewSink_ExistingGroupAndStream(t *testing.T) {
	exists := &types.ResourceAlreadyExistsException{Message: aws.String("exists")}
	api := &fakeAPI{createGroupErr: exists, createStreamErr: exists}

	s := newSink(t, api, cloudwatch.WithCreateLogGroup())
	require.NoError(t, s.Close())
}

func TestNewSink_CreateStreamFails(t *testing.T) {
	api := &fakeAPI{createStreamErr: errors.New("AccessDeniedException")}

	_, err := cloudwatch.NewSink("group", "stream", cloudwatch.WithClient(api))
	assert.ErrorContains(t, err, `cloudwatch: create log stream "stream": AccessDeniedException`)
}

func TestNewSink_CreateGroupFails(t *testing.T) {
	api := &fakeAPI{createGroupErr: errors.New("AccessDeniedException")}

	_, err := cloudwatch.NewSink("group", "stream",
		cloudwatch.WithClient(api),
		cloudwatch.WithCreateLogGroup(),
	)
	assert.ErrorContains(t, err, `cloudwatch: create log group "group": AccessDeniedException`)
}

func TestSink_SendsEvents(t *testing.T) {
	api := &fakeAPI{}
	s := newSink(t, api)
	defer s.Close()

	before := time.Now().UnixMilli()
	n, err := s.Write([]byte(`{"level":"info","msg":"hello"}` + "\n"))
	require.NoError(t, err)
	assert.Equal(t, 31, n)
	_, err = s.Write([]byte("plain\r\n"))
	require.NoError(t, err)
	require.NoError(t, s.(interface{ Sync() error }).Sync())
	after := time.Now().UnixMilli()

	require.Equal(t, 1, api.batchCount())
	batch := api.batches[0]
	require.Len(t, batch, 2)
	assert.Equal(t, `{"level":"info","msg":"hello"}`, aws.ToString(batch[0].Message))
	assert.Equal(t, "plain", aws.ToString(batch[1].Message))
	for _, ev := range batch {
		ts := aws.ToInt64(ev.Timestamp)
		assert.GreaterOrEqual(t, ts, before)
		assert.LessOrEqual(t, ts, after)
	}
}

func TestSink_EmptyLineSkipped(t *testing.T) {
	api := &fakeAPI{}
	s := newSink(t, api)

	n, err := s.Write([]byte("\n"))
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.NoError(t, s.Close())

	assert.Equal(t, 0, api.batchCount())
}

func TestSink_BatchesByCount(t *testing.T) {
	api := &fakeAPI{}
	s := newSink(t, api, cloudwatch.WithBatchSize(3))

	for i := range 7 {
		_, err := s.Write(fmt.Appendf(nil, "line %d", i))
		require.NoError(t, err)
	}
	require.NoError(t, s.Close())

	require.Equal(t, 3, api.batchCount())
	assert.Len(t, api.batches[0], 3)
	assert.Len(t, api.batches[1], 3)
	assert.Len(t, api.batches[2], 1)
}

func TestSink_SplitsBatchesBySize(t *testing.T) {
	api := &fakeAPI{}
	s := newSink(t, api)

	// Three 400 KB events exceed the 1 MiB batch limit, so they need two calls.
	line := strings.Repeat("x", 400*1024)
	for range 3 {
		_, err := s.Write([]byte(line))
		require.NoError(t, err)
	}
	require.NoError(t, s.Close())

	require.Equal(t, 2, api.batchCount())
	assert.Len(t, api.batches[0], 2)
	assert.Len(t, api.batches[1], 1)
}

func TestSink_TruncatesOversizedEvent(t *testing.T) {
	api := &fakeAPI{}
	s := newSink(t, api)

	_, err := s.Write([]byte(strings.Repeat("ä", 600*1024))) // 1.2 MB
	require.NoError(t, err)
	require.NoError(t, s.Close())

	msgs := api.messages()
	require.Len(t, msgs, 1)
	assert.LessOrEqual(t, len(msgs[0])+26, 1048576)
	assert.True(t, strings.HasSuffix(msgs[0], "ä"), "must be cut at a rune boundary")
}

func TestSink_FlushesOnInterval(t *testing.T) {
	api := &fakeAPI{}
	s, err := cloudwatch.NewSink("group", "stream",
		cloudwatch.WithClient(api),
		cloudwatch.WithFlushInterval(20*time.Millisecond),
	)
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Write([]byte("tick"))
	require.NoError(t, err)

	assert.Eventually(t, func() bool { return api.batchCount() == 1 }, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, []string{"tick"}, api.messages())
}

func TestSink_RejectedEvents(t *testing.T) {
	api := &fakeAPI{putOut: &cloudwatchlogs.PutLogEventsOutput{
		RejectedLogEventsInfo: &types.RejectedLogEventsInfo{
			TooOldLogEventEndIndex:   aws.Int32(1),
			TooNewLogEventStartIndex: aws.Int32(4),
		},
	}}
	s := newSink(t, api)
	defer s.Close()

	for i := range 5 {
		_, err := s.Write(fmt.Appendf(nil, "line %d", i))
		require.NoError(t, err)
	}

	err := s.(interface{ Sync() error }).Sync()
	assert.ErrorIs(t, err, cloudwatch.ErrRejected)
	assert.ErrorContains(t, err, "2 too old, 1 too new")
}

func TestSink_RecreatesDeletedStream(t *testing.T) {
	api := &fakeAPI{putErr: func(n int) error {
		if n == 0 {
			return &types.ResourceNotFoundException{Message: aws.String("stream deleted")}
		}
		return nil
	}}
	s := newSink(t, api)
	defer s.Close()

	_, err := s.Write([]byte("after delete"))
	require.NoError(t, err)
	require.NoError(t, s.(interface{ Sync() error }).Sync())

	assert.Equal(t, []string{"group/stream", "group/stream"}, api.streams)
	assert.Equal(t, 2, api.batchCount())
}

func TestSink_PutErrorReturnedWithoutHandler(t *testing.T) {
	api := &fakeAPI{putErr: func(int) error { return errors.New("ThrottlingException") }}
	s := newSink(t, api)

	_, err := s.Write([]byte("one"))
	require.NoError(t, err)
	assert.ErrorContains(t, s.(interface{ Sync() error }).Sync(), "cloudwatch: put log events: ThrottlingException")

	_, err = s.Write([]byte("two"))
	require.NoError(t, err)
	assert.ErrorContains(t, s.Close(), "ThrottlingException")
}

func TestSink_BackgroundErrorReportedToHandler(t *testing.T) {
	api := &fakeAPI{putErr: func(int) error { return errors.New("ThrottlingException") }}

	errs := make(chan error, 1)
	s := newSink(t, api,
		cloudwatch.WithBatchSize(1),
		cloudwatch.WithErrorHandler(func(err error) {
			select {
			case errs <- err:
			default:
			}
		}),
	)
	defer s.Close()

	_, err := s.Write([]byte("fails"))
	require.NoError(t, err)

	select {
	case err := <-errs:
		assert.ErrorContains(t, err, "ThrottlingException")
	case <-time.After(2 * time.Second):
		require.Fail(t, "background delivery error was not reported")
	}
}

func TestSink_WriteDoesNotBlockAndDropsWhenFull(t *testing.T) {
	api := &fakeAPI{release: make(chan struct{})}
	s := newSink(t, api,
		cloudwatch.WithBatchSize(1),
		cloudwatch.WithQueueSize(2),
	)

	start := time.Now()
	var dropped int
	for i := range 10 {
		if _, err := s.Write(fmt.Appendf(nil, "line %d", i)); err != nil {
			assert.ErrorIs(t, err, cloudwatch.ErrQueueFull)
			dropped++
		}
	}
	assert.Less(t, time.Since(start), 500*time.Millisecond)
	assert.GreaterOrEqual(t, dropped, 7)

	close(api.release)
	require.NoError(t, s.Close())
}

func TestSink_CloseDeliversQueuedLines(t *testing.T) {
	api := &fakeAPI{}
	s := newSink(t, api, cloudwatch.WithBatchSize(7))

	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 25 {
				_, err := s.Write(fmt.Appendf(nil, "g%d line %d", g, i))
				assert.NoError(t, err)
			}
		})
	}
	wg.Wait()
	require.NoError(t, s.Close())

	assert.Len(t, api.messages(), 100)

	// Every batch must be in chronological order.
	for _, b := range api.batches {
		for i := 1; i < len(b); i++ {
			assert.LessOrEqual(t, aws.ToInt64(b[i-1].Timestamp), aws.ToInt64(b[i].Timestamp))
		}
	}
}

func TestSink_WriteAfterClose(t *testing.T) {
	s := newSink(t, &fakeAPI{})
	require.NoError(t, s.Close())
	require.NoError(t, s.Close())

	_, err := s.Write([]byte("late"))
	assert.ErrorIs(t, err, cloudwatch.ErrClosed)
	assert.NoError(t, s.(interface{ Sync() error }).Sync())
}

func TestWithCloudWatch(t *testing.T) {
	api := &fakeAPI{}
	opt, err := cloudwatch.WithCloudWatch("group", "stream", cloudwatch.WithClient(api))
	require.NoError(t, err)

	log := zaplogger.NewLogger(zaplogger.WithOutput(zaplogger.JSONOutput), opt)
	log.Info("from zap")
	require.NoError(t, log.Sync())

	msgs := api.messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], `"msg":"from zap"`)
}

func TestWithCloudWatch_Error(t *testing.T) {
	_, err := cloudwatch.WithCloudWatch("group", "stream",
		cloudwatch.WithClient(&fakeAPI{createStreamErr: errors.New("denied")}),
	)
	assert.ErrorContains(t, err, "denied")
}

// TestSink_AWSConfig drives the real SDK client against a local server that
// speaks the CloudWatch Logs JSON protocol.
func TestSink_AWSConfig(t *testing.T) {
	var (
		mu      sync.Mutex
		targets []string
		events  []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.Header.Get("X-Amz-Target")
		assert.Contains(t, r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AKID/")

		mu.Lock()
		targets = append(targets, target)
		if target == "Logs_20140328.PutLogEvents" {
			var in struct {
				LogEvents []struct{ Message string }
			}
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&in))
			for _, ev := range in.LogEvents {
				events = append(events, ev.Message)
			}
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cfg := aws.Config{
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		BaseEndpoint: aws.String(srv.URL),
	}

	s, err := cloudwatch.NewSink("group", "stream",
		cloudwatch.WithAWSConfig(cfg),
		cloudwatch.WithCreateLogGroup(),
	)
	require.NoError(t, err)

	_, err = s.Write([]byte("via sdk\n"))
	require.NoError(t, err)
	require.NoError(t, s.Close())

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{
		"Logs_20140328.CreateLogGroup",
		"Logs_20140328.CreateLogStream",
		"Logs_20140328.PutLogEvents",
	}, targets)
	assert.Equal(t, []string{"via sdk"}, events)
}

// newLogsServer serves the CloudWatch Logs JSON protocol, answering every
// call with an empty result. With hang set, PutLogEvents never answers.
func newLogsServer(t *testing.T, hang bool) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The server only notices a client disconnect once the body is read.
		io.Copy(io.Discard, r.Body)
		if hang && r.Header.Get("X-Amz-Target") == "Logs_20140328.PutLogEvents" {
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})
	return srv
}

// TestSink_DefaultConfigChain uses the default AWS configuration chain, fed
// from environment variables only.
func TestSink_DefaultConfigChain(t *testing.T) {
	srv := newLogsServer(t, false)

	dir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", dir+"/config")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", dir+"/credentials")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_REGION", "eu-west-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_ENDPOINT_URL_CLOUDWATCH_LOGS", srv.URL)

	s, err := cloudwatch.NewSink("group", "stream")
	require.NoError(t, err)

	_, err = s.Write([]byte("default chain"))
	require.NoError(t, err)
	assert.NoError(t, s.Close())
}

func TestSink_Timeout(t *testing.T) {
	srv := newLogsServer(t, true)

	s, err := cloudwatch.NewSink("group", "stream",
		cloudwatch.WithAWSConfig(aws.Config{
			Region:       "us-east-1",
			Credentials:  credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
			BaseEndpoint: aws.String(srv.URL),
		}),
		cloudwatch.WithTimeout(100*time.Millisecond),
	)
	require.NoError(t, err)

	_, err = s.Write([]byte("slow"))
	require.NoError(t, err)

	start := time.Now()
	err = s.Close()
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestSink_RecreateStreamFails(t *testing.T) {
	api := &fakeAPI{putErr: func(int) error {
		return &types.ResourceNotFoundException{Message: aws.String("group deleted")}
	}}
	s := newSink(t, api)

	api.mu.Lock()
	api.createStreamErr = errors.New("group does not exist")
	api.mu.Unlock()

	_, err := s.Write([]byte("lost"))
	require.NoError(t, err)

	err = s.Close()
	assert.ErrorContains(t, err, "group deleted")
	assert.ErrorContains(t, err, "group does not exist")
	assert.Equal(t, 1, api.batchCount(), "no retry after the stream could not be recreated")
}

func TestNewSink_ExistingLogStream(t *testing.T) {
	api := &fakeAPI{createStreamErr: errors.New("AccessDeniedException")}
	s := newSink(t, api, cloudwatch.WithExistingLogStream())

	_, err := s.Write([]byte("only put"))
	require.NoError(t, err)
	require.NoError(t, s.Close())

	assert.Empty(t, api.groups)
	assert.Empty(t, api.streams)
	assert.Equal(t, []string{"only put"}, api.messages())
}

func TestNewSink_ExistingLogStreamWithCreateGroup(t *testing.T) {
	api := &fakeAPI{}
	s := newSink(t, api, cloudwatch.WithExistingLogStream(), cloudwatch.WithCreateLogGroup())
	require.NoError(t, s.Close())

	assert.Equal(t, []string{"group"}, api.groups)
	assert.Empty(t, api.streams)
}

func TestSink_ExistingLogStreamNotRecreated(t *testing.T) {
	api := &fakeAPI{putErr: func(int) error {
		return &types.ResourceNotFoundException{Message: aws.String("stream missing")}
	}}
	s := newSink(t, api, cloudwatch.WithExistingLogStream())

	_, err := s.Write([]byte("lost"))
	require.NoError(t, err)

	err = s.Close()
	assert.ErrorContains(t, err, "cloudwatch: put log events:")
	assert.ErrorContains(t, err, "stream missing")
	assert.Empty(t, api.streams)
	assert.Equal(t, 1, api.batchCount(), "no retry without recreating the stream")
}
