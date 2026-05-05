package datadog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/zauberhaus/logger/pkg/logger"
)

// Writer uses the official Datadog Go SDK to forward logs via the typed SubmitLog API.
type Sink struct {
	api     *datadogV2.LogsApi
	authCtx context.Context // pre-built with API key auth, reused for every call

	service    string
	host       string
	source     string
	tags       string
	httpClient *http.Client // optional; replaces the SDK's default HTTP client

	batchSize     int
	flushInterval time.Duration
	onError       func(error)

	mu    sync.Mutex
	lines [][]byte
	size  int

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewWriter creates a Writer that forwards logs to Datadog using the official Go SDK.
// It validates the API key before returning; an invalid or unreachable key returns an error.
// Call Close when done to flush pending logs and stop the background flusher.
func NewSink(apiKey string, opts ...Option) (logger.Sink, error) {
	w := &Sink{
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
		done:          make(chan struct{}),
	}
	for _, opt := range opts {
		opt.apply(w)
	}

	cfg := datadog.NewConfiguration()
	if w.httpClient != nil {
		cfg.HTTPClient = w.httpClient
	}
	client := datadog.NewAPIClient(cfg)
	w.api = datadogV2.NewLogsApi(client)
	w.authCtx = context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{"apiKeyAuth": {Key: apiKey}},
	)

	resp, _, err := datadogV1.NewAuthenticationApi(client).Validate(w.authCtx)
	if err != nil {
		return nil, fmt.Errorf("datadog: health check: %w", err)
	}
	if !resp.GetValid() {
		return nil, fmt.Errorf("datadog: invalid API key")
	}

	w.ctx, w.cancel = context.WithCancel(context.Background())
	go w.run()

	return w, nil
}

// Write buffers a log line. Implements io.Writer and zapcore.WriteSyncer.
func (w *Sink) Write(p []byte) (int, error) {
	line := bytes.TrimRight(p, "\n")
	if len(line) == 0 {
		return len(p), nil
	}
	dst := make([]byte, len(line))
	copy(dst, line)

	w.mu.Lock()
	w.lines = append(w.lines, dst)
	w.size += len(dst)
	flush := len(w.lines) >= w.batchSize || w.size >= maxPayloadBytes
	w.mu.Unlock()

	if flush {
		err := w.flush()
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Sync flushes buffered logs immediately. Implements zapcore.WriteSyncer.
func (w *Sink) Sync() error {
	return w.flush()
}

// Close flushes remaining logs and stops the background flusher.
func (w *Sink) Close() error {
	w.cancel()
	<-w.done
	return w.flush()
}

func (w *Sink) run() {
	defer close(w.done)
	t := time.NewTicker(w.flushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			w.flush()
		case <-w.ctx.Done():
			return
		}
	}
}

func (w *Sink) flush() error {
	w.mu.Lock()
	if len(w.lines) == 0 {
		w.mu.Unlock()
		return nil
	}
	lines := w.lines
	w.lines = nil
	w.size = 0
	w.mu.Unlock()

	items := w.buildItems(lines)
	_, r, err := w.api.SubmitLog(w.authCtx, items, *datadogV2.NewSubmitLogOptionalParameters())
	if err != nil {
		return w.handleError(fmt.Errorf("datadog: submit logs: %w", err))
	}
	defer r.Body.Close()
	if r.StatusCode != 202 {
		return w.handleError(fmt.Errorf("datadog: HTTP %d", r.StatusCode))
	}

	return nil
}

// buildItems converts buffered log lines into Datadog HTTPLogItem values.
// For JSON lines (zap JSONOutput), msg is extracted as Message and remaining
// fields are forwarded as AdditionalProperties for structured querying in Datadog.
func (w *Sink) buildItems(lines [][]byte) []datadogV2.HTTPLogItem {
	items := make([]datadogV2.HTTPLogItem, 0, len(lines))
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		item := datadogV2.HTTPLogItem{}

		var fields map[string]any
		if err := json.Unmarshal(trimmed, &fields); err == nil {
			if msg, ok := fields["msg"].(string); ok {
				item.Message = msg
				delete(fields, "msg")
			} else {
				item.Message = string(trimmed)
			}
			if len(fields) > 0 {
				item.AdditionalProperties = fields
			}
		} else {
			item.Message = string(trimmed)
		}

		if w.service != "" {
			item.Service = datadog.PtrString(w.service)
		}
		if w.host != "" {
			item.Hostname = datadog.PtrString(w.host)
		}
		if w.source != "" {
			if item.AdditionalProperties == nil {
				item.AdditionalProperties = make(map[string]any)
			}
			item.AdditionalProperties["ddsource"] = w.source
		}
		if w.tags != "" {
			item.Ddtags = datadog.PtrString(w.tags)
		}

		items = append(items, item)
	}
	return items
}

func (w *Sink) handleError(err error) error {
	if w.onError != nil {
		w.onError(err)
		return nil
	}

	return err
}
