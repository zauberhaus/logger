// cspell:ignore andybalholm
package http_logger

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/zstd"
	"github.com/andybalholm/brotli"
	"github.com/zauberhaus/logger"
)

var ErrUnknownContentEncoding = errors.New("unknown content encoding")

type LoggingTransport struct {
	proxied http.RoundTripper
	logger  logger.Logger
}

func NewLoggingTransport(proxied http.RoundTripper, logger logger.Logger) http.RoundTripper {
	return &LoggingTransport{
		proxied: proxied,
		logger:  logger,
	}
}

// maxLoggedBody caps how many bytes of a request or response body are
// buffered and logged. Larger bodies are logged truncated and streamed through.
const maxLoggedBody = 1 << 20

// RoundTrip executes a single HTTP transaction and logs the details
func (l *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if l.logger == nil || !l.logger.IsDebugEnabled() {
		return l.proxied.RoundTrip(req)
	}

	url := req.URL.String()

	l.logger.Debugf("--> %s %s", req.Method, url)

	if req.Body != nil {
		body, complete, err := peekBody(req.Body)
		if err != nil {
			req.Body.Close()
			return nil, err
		}

		if complete {
			req.Body.Close()
			req.Body = io.NopCloser(bytes.NewReader(body))
		} else {
			req.Body = &multiReadCloser{
				Reader:  io.MultiReader(bytes.NewReader(body), req.Body),
				closers: []io.Closer{req.Body},
			}
		}

		l.logBody("request body", body, complete)
	}

	start := time.Now()

	resp, err := l.proxied.RoundTrip(req)

	duration := time.Since(start)
	if err != nil {
		l.logger.Errorf("<-- ERROR %s %s: %v (%s)", req.Method, req.URL.String(), err, duration)
		return nil, err
	}

	l.logger.Debugf("<-- %d %s (%s)", resp.StatusCode, url, duration)

	if resp.Body != nil {
		reader, decoded, err := ProcessEncoding(resp)
		if err != nil && err != ErrUnknownContentEncoding {
			resp.Body.Close()
			return nil, err
		}

		// Decoders do not close the underlying body, so close both.
		closers := []io.Closer{resp.Body}
		if decoded {
			closers = append([]io.Closer{reader}, closers...)
		}
		body, complete, err := peekBody(reader)
		if err != nil {
			closeAll(closers)
			return nil, err
		}

		if complete {
			closeAll(closers)
			resp.Body = io.NopCloser(bytes.NewReader(body))
		} else {
			resp.Body = &multiReadCloser{
				Reader:  io.MultiReader(bytes.NewReader(body), reader),
				closers: closers,
			}
		}

		if decoded {
			resp.Header.Del("Content-Encoding")
			if complete {
				resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
				resp.ContentLength = int64(len(body))
			} else {
				resp.Header.Del("Content-Length")
				resp.ContentLength = -1
			}
		}

		l.logBody("response body", body, complete)
	}

	return resp, nil
}

func (l *LoggingTransport) logBody(name string, body []byte, complete bool) {
	if len(body) == 0 {
		return
	}

	if !complete {
		body = body[:maxLoggedBody]
		name += " (truncated)"
	}

	if IsPrintableFast(body) {
		l.logger.Debugf("%s: %s", name, body)
	} else {
		l.logger.Debugf("%s:\n%s", name, hex.Dump(body))
	}
}

// peekBody reads up to maxLoggedBody bytes from r. complete reports whether
// r was fully consumed; if not, the returned prefix holds one byte more than
// maxLoggedBody and the rest is still unread in r.
func peekBody(r io.Reader) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxLoggedBody+1))
	if err != nil {
		return nil, false, err
	}
	return body, len(body) <= maxLoggedBody, nil
}

type multiReadCloser struct {
	io.Reader
	closers []io.Closer
}

func (m *multiReadCloser) Close() error {
	return closeAll(m.closers)
}

func closeAll(closers []io.Closer) error {
	var errs []error
	for _, c := range closers {
		errs = append(errs, c.Close())
	}
	return errors.Join(errs...)
}

func ProcessEncoding(resp *http.Response) (io.ReadCloser, bool, error) {
	if resp.Body == nil {
		return resp.Body, false, nil
	}

	encoding := resp.Header.Get("Content-Encoding")
	switch encoding {
	case "br":
		return io.NopCloser(brotli.NewReader(resp.Body)), true, nil
	case "gzip", "gz":
		r, err := gzip.NewReader(resp.Body)
		return r, true, err
	case "deflate":
		return flate.NewReader(resp.Body), true, nil
	case "zstd":
		return zstd.NewReader(resp.Body), true, nil
	case "", "identity":
		return resp.Body, false, nil
	}

	return resp.Body, false, ErrUnknownContentEncoding
}

func IsPrintableFast(data []byte) bool {
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError {
			return false
		}
		if !unicode.IsPrint(r) && !unicode.IsSpace(r) {
			return false
		}
		i += size
	}
	return true
}
