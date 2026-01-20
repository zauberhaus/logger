package memory_test

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/memory"
	"github.com/zauberhaus/logger/pkg/zap"
)

func TestMemoryLogger(t *testing.T) {
	t.Parallel()
	type message struct {
		Level      logger.Level
		Message    string    `json:"msg"`
		Time       time.Time `json:"ts"`
		Number     int       `json:"number"`
		Caller     string    `json:"caller"`
		Stacktrace string    `json:"stacktrace"`
	}

	l := memory.NewLogger(zap.WithOutput(zap.JSONOutput), zap.WithField("number", 99))
	defer l.Close()

	l.Info("test message 1")
	l.Error("test message 2")

	line := l.NextLine()

	var msg message

	decoder := json.NewDecoder(bytes.NewBuffer([]byte(line)))
	decoder.DisallowUnknownFields()

	err := decoder.Decode(&msg)
	if assert.NoError(t, err) {
		assert.Equal(t, logger.InfoLevel, msg.Level)
		assert.Equal(t, "test message 1", msg.Message)
		assert.Equal(t, 99, msg.Number)
		assert.Empty(t, msg.Stacktrace)
		assert.Contains(t, msg.Caller, "memory/logger_test")
	}

	line = l.NextLine()

	decoder = json.NewDecoder(bytes.NewBuffer([]byte(line)))
	decoder.DisallowUnknownFields()

	err = decoder.Decode(&msg)
	if assert.NoError(t, err) {
		assert.Equal(t, logger.ErrorLevel, msg.Level)
		assert.Equal(t, "test message 2", msg.Message)
		assert.Equal(t, 99, msg.Number)
		assert.NotEmpty(t, msg.Stacktrace)
		assert.Contains(t, msg.Caller, "memory/logger_test")
	}
}

func TestMemoryLogger_Blocking(t *testing.T) {
	t.Parallel()
	l := memory.NewLogger(memory.WithBlocking(true))
	defer l.Close()

	lineChan := make(chan string)

	go func() {
		line := l.NextLine()
		lineChan <- line
	}()

	time.Sleep(100 * time.Millisecond)

	l.Info("blocking test message")

	select {
	case line := <-lineChan:
		assert.Contains(t, line, "blocking test message")
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out, NextLine did not unblock")
	}
}

func TestMemoryLogger_Close(t *testing.T) {
	t.Parallel()
	l := memory.NewLogger()

	lineChan := make(chan string)

	go func() {
		line := l.NextLine()
		lineChan <- line
	}()

	l.Close()

	select {
	case line := <-lineChan:
		assert.Empty(t, line, "NextLine should return empty string after Close")
	case <-time.After(1 * time.Second):
		t.Fatal("Test timed out, NextLine did not unblock after Close")
	}
}

func TestMemoryLogger_Channel(t *testing.T) {
	for _, closed := range []bool{true, false} {
		name := "closed"
		if !closed {
			name = "not closed"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, blocking := range []bool{true, false} {
				name := "blocking"
				if !blocking {
					name = "not blocking"
				}

				t.Run(name, func(t *testing.T) {
					t.Parallel()
					l := memory.NewLogger(memory.WithBlocking(blocking))

					errChan := make(chan error, 1)
					lineChan, err := l.LineChannel()
					if err != nil {
						errChan <- err
					} else {
						lineChan2, err := l.LineChannel()
						if err != nil {
							errChan <- err
						}

						assert.Equal(t, lineChan, lineChan2)

					}

					time.Sleep(100 * time.Millisecond)
					l.Info("test message")

					if closed {
						l.Close()
					}

					select {
					case err := <-errChan:
						if blocking {
							assert.NoError(t, err)
						} else {
							assert.ErrorContains(t, err, "LineChannel only works with blocking mode")
						}
					case line := <-lineChan:
						if blocking {
							assert.Contains(t, line, "test message")
						} else {
							assert.Fail(t, "Non-blocking Read shouldn't return data after Close")
						}
					case <-time.After(1 * time.Second):
						t.Fatal("Test timed out, Read did not unblock after Close")
					}
				})
			}
		})
	}
}

func TestMemoryLogger_Read(t *testing.T) {
	t.Parallel()
	for _, closed := range []bool{true, false} {
		name := "closed"
		if !closed {
			name = "not closed"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, blocking := range []bool{true, false} {
				name := "blocking"
				if !blocking {
					name = "not blocking"
				}

				t.Run(name, func(t *testing.T) {
					t.Parallel()
					l := memory.NewLogger(memory.WithBlocking(blocking))

					lineChan := make(chan string)
					errChan := make(chan error)

					go func() {
						data := make([]byte, 1024)

						r := l.Reader()
						n, err := r.Read(data)

						if err != nil {
							errChan <- err
						} else {
							lineChan <- string(data[:n])
						}
					}()

					time.Sleep(100 * time.Millisecond)
					l.Info("test message")

					if closed {
						l.Close()
					}

					select {
					case err := <-errChan:
						if blocking {
							assert.NoError(t, err)
						} else {
							assert.ErrorContains(t, err, "ringbuffer is empty")
						}

					case line := <-lineChan:
						if blocking {
							assert.Contains(t, line, "test message")
						} else {
							assert.Fail(t, "Non-blocking Read shouldn't return data after Close")
						}
					case <-time.After(1 * time.Second):
						t.Fatal("Test timed out, Read did not unblock after Close")
					}
				})
			}
		})
	}
}

func TestMemoryLogger_ReadAll(t *testing.T) {
	t.Parallel()

	for _, closed := range []bool{true, false} {
		name := "closed"
		if !closed {
			name = "not closed"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, blocking := range []bool{true, false} {
				name := "blocking"
				if !blocking {
					name = "not blocking"
				}

				t.Run(name, func(t *testing.T) {
					t.Parallel()
					l := memory.NewLogger(memory.WithBlocking(blocking))

					blocked := !closed && blocking

					lineChan := make(chan string)
					errChan := make(chan error)

					go func() {
						all, err := io.ReadAll(l.Reader())

						if err != nil {
							errChan <- err
						} else {
							lineChan <- string(all)
						}
					}()

					time.Sleep(100 * time.Millisecond)
					l.Info("test message")

					if closed {
						l.Close()
					}

					select {
					case err := <-errChan:
						if blocked {
							assert.Fail(t, "Blocking ReadAll without Close should not return error")
						} else {
							if blocking {
								assert.NoError(t, err)
							} else {
								assert.ErrorContains(t, err, "ringbuffer is empty")
							}
						}

					case line := <-lineChan:
						if blocked {
							assert.Fail(t, "Blocking ReadAll without Close should not return data")
						} else {
							if blocking {
								assert.Contains(t, line, "test message")
							} else {
								assert.Fail(t, "Non-blocking ReadAll shouldn't return data after Close")
							}
						}
					case <-time.After(1 * time.Second):
						if !(blocked) {
							if closed && blocking {
								t.Fatal("Test timed out, ReadAll did not unblock after Close")
							} else {
								t.Fatal("Test timed out, ReadAll did not unblock")
							}
						}
					}
				})
			}
		})
	}
}

func TestMemoryLogger_BufferSize(t *testing.T) {
	t.Parallel()

	for _, blocking := range []bool{true, false} {
		name := "blocking"
		if !blocking {
			name = "not blocking"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var errBuff bytes.Buffer

			buffSize := 8

			// A small buffer that can hold a few messages
			l := memory.NewLogger(memory.WithBufferSize(buffSize), memory.WithBlocking(blocking), zap.WithErrorSinks(&errBuff))
			defer l.Close()

			var wg sync.WaitGroup
			wg.Add(1)

			// This goroutine should block after a few messages
			go func() {
				defer wg.Done()
				for range 10 {
					l.Info("test message")
				}

				l.Close()
			}()

			// Give the goroutine time to fill the buffer and block
			time.Sleep(100 * time.Millisecond)

			// Now, read from the buffer, which should unblock the writer
			for i := range 10 {
				line := l.NextLine()

				if blocking {
					assert.Contains(t, line, "test message")
				} else {
					if i == 0 {
						assert.Len(t, line, 8)
					} else {
						assert.Empty(t, line)
					}
				}
			}

			wg.Wait()

			output := errBuff.String()
			if !blocking {
				assert.Contains(t, output, "too much data to write")
				assert.Contains(t, output, "ringbuffer is full")
			} else {
				assert.Empty(t, output)
			}
		})
	}
}

func TestMemoryLogger_Sync(t *testing.T) {

	l := memory.NewLogger(memory.WithBlocking(true))
	l.Info("test message")

	go func() {
		line := l.NextLine()
		assert.NotEmpty(t, line)
	}()

	// wait until buffer is emptied
	l.Sync()

	assert.Len(t, l.Bytes(), 0)

}

func TestMemoryLogger_Reset(t *testing.T) {

	l := memory.NewLogger(memory.WithBlocking(true), memory.WithBufferSize(128))
	l.Info("test message")

	len1 := len(l.Bytes())

	assert.Greater(t, len1, 0)
	assert.Equal(t, len1, l.Len())
	assert.Equal(t, 128-len1, l.Free())

	l.Reset()

	assert.Len(t, l.Bytes(), 0)
	assert.Equal(t, 0, l.Len())
	assert.Equal(t, 128, l.Free())

}
