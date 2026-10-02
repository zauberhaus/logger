// cspell:ignore xffb
package cloudwatch

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncate(t *testing.T) {
	tests := map[string]struct {
		in   string
		max  int
		want string
	}{
		"short":             {in: "hello", max: 10, want: "hello"},
		"exact":             {in: "hello", max: 5, want: "hello"},
		"ascii cut":         {in: "hello", max: 3, want: "hel"},
		"rune boundary":     {in: "aä", max: 2, want: "a"},
		"whole rune fits":   {in: "aäb", max: 3, want: "aä"},
		"invalid replaced":  {in: "a\xffb", max: 10, want: "a�b"},
		"replacement fits":  {in: "a\xffb", max: 4, want: "a�"},
		"replacement split": {in: "a\xffb", max: 3, want: "a"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := truncate([]byte(tt.in), tt.max)
			assert.Equal(t, tt.want, got)
			assert.True(t, utf8.ValidString(got))
			assert.LessOrEqual(t, len(got), tt.max)
		})
	}
}

type noopAPI struct{}

func (noopAPI) PutLogEvents(context.Context, *cloudwatchlogs.PutLogEventsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error) {
	return &cloudwatchlogs.PutLogEventsOutput{}, nil
}

func (noopAPI) CreateLogStream(context.Context, *cloudwatchlogs.CreateLogStreamInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	return &cloudwatchlogs.CreateLogStreamOutput{}, nil
}

func (noopAPI) CreateLogGroup(context.Context, *cloudwatchlogs.CreateLogGroupInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	return &cloudwatchlogs.CreateLogGroupOutput{}, nil
}

type recordingAPI struct {
	API
	batches [][]types.InputLogEvent
}

func (r *recordingAPI) PutLogEvents(_ context.Context, in *cloudwatchlogs.PutLogEventsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error) {
	r.batches = append(r.batches, in.LogEvents)
	return &cloudwatchlogs.PutLogEventsOutput{}, nil
}

func TestFlush_SortsChronologically(t *testing.T) {
	api := &recordingAPI{}
	w := &Sink{client: api, group: "g", stream: "s", batchSize: 100, timeout: time.Second}

	w.add(event{timestamp: 30, message: "c"})
	w.add(event{timestamp: 10, message: "a"})
	w.add(event{timestamp: 20, message: "b"})
	require.NoError(t, w.flush())

	require.Len(t, api.batches, 1)
	var got []string
	for _, ev := range api.batches[0] {
		got = append(got, aws.ToString(ev.Message))
	}
	assert.Equal(t, []string{"a", "b", "c"}, got)
}

func TestAdd_SplitsBatchSpanningMoreThan24h(t *testing.T) {
	api := &recordingAPI{}
	w := &Sink{client: api, group: "g", stream: "s", batchSize: 100, timeout: time.Second}

	start := time.Now().Add(-25 * time.Hour).UnixMilli()
	w.add(event{timestamp: start, message: "old"})
	w.add(event{timestamp: start + time.Hour.Milliseconds(), message: "still in span"})
	w.add(event{timestamp: start + 25*time.Hour.Milliseconds(), message: "new"})
	require.NoError(t, w.flush())

	require.Len(t, api.batches, 2)
	assert.Len(t, api.batches[0], 2)
	assert.Len(t, api.batches[1], 1)
}

func TestRejected(t *testing.T) {
	assert.NoError(t, rejected(nil, 5))
	assert.NoError(t, rejected(&types.RejectedLogEventsInfo{}, 5))

	err := rejected(&types.RejectedLogEventsInfo{ExpiredLogEventEndIndex: aws.Int32(2)}, 5)
	assert.ErrorIs(t, err, ErrRejected)
	assert.EqualError(t, err, "cloudwatch: log events rejected: 3 expired")
}

func TestNewSink_BatchSizeCapped(t *testing.T) {
	for _, n := range []int{0, -1, maxBatchEvents + 1} {
		w, err := NewSink("g", "s", WithClient(noopAPI{}), WithBatchSize(n))
		require.NoError(t, err)
		assert.Equal(t, maxBatchEvents, w.(*Sink).batchSize)
		require.NoError(t, w.Close())
	}
}

// Ensure the large-message path never exceeds the per-event limit.
func TestTruncate_MaxEventBytes(t *testing.T) {
	got := truncate([]byte(strings.Repeat("€", maxEventBytes)), maxEventBytes)
	assert.LessOrEqual(t, len(got), maxEventBytes)
	assert.True(t, utf8.ValidString(got))
}
