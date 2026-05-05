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

// RoundTrip executes a single HTTP transaction and logs the details
func (l *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {

	if l.logger != nil && l.logger.IsDebugEnabled() {

		l.logger.Debugf("--> %s %s", req.Method, req.URL.String())

		if req.Body != nil {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			defer req.Body.Close()

			l.logger.Debugf("request body: %s", body)

			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		start := time.Now()

		resp, err := l.proxied.RoundTrip(req)

		duration := time.Since(start)
		if err != nil {
			l.logger.Errorf("<-- ERROR %s %s: %v (%s)", req.Method, req.URL.String(), err, duration)
			return nil, err
		}

		l.logger.Debugf("<-- %d %s (%s)", resp.StatusCode, req.URL.String(), duration)

		if resp.Body != nil {
			reader, decoded, err := ProcessEncoding(resp)
			if err != nil {
				if err != ErrUnknownContentEncoding {
					return resp, err
				}

			}
			body, err := io.ReadAll(reader)
			if err != nil {
				return resp, err
			}
			defer reader.Close()

			resp.Body = io.NopCloser(bytes.NewReader(body))

			if decoded {
				resp.Header.Del("Content-Encoding")
				resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
			}

			if len(body) > 0 {
				if IsPrintableFast(body) {
					l.logger.Debugf("response body: %s", body)

				} else {
					l.logger.Debugf("response body:\n%s", hex.Dump(body))
				}
			}

		}

		return resp, nil
	} else {
		return l.proxied.RoundTrip(req)
	}
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
