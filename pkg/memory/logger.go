package memory

import (
	"bufio"
	"fmt"
	"io"
	"sync"

	"github.com/zauberhaus/logger"
	"github.com/zauberhaus/logger/pkg/zap"

	"github.com/smallnest/ringbuffer"
)

var _ logger.Logger = &MemoryLogger{}

func NewLogger(options ...zap.Option) *MemoryLogger {
	size := BufferSize(options...)
	blocking := Blocking(options...)

	buffer := ringbuffer.New(size)
	if blocking {
		buffer = buffer.SetBlocking(true)
	}

	options = append(options, zap.WithSink(buffer))

	return &MemoryLogger{
		Logger:   zap.NewLogger(options...),
		buffer:   buffer,
		scanner:  bufio.NewScanner(buffer),
		blocking: blocking,
	}
}

type MemoryLogger struct {
	logger.Logger
	buffer *ringbuffer.RingBuffer

	scanner *bufio.Scanner

	blocking bool
	running  bool
	lineChan chan string
	lock     sync.Mutex
}

func (m *MemoryLogger) Close() {
	m.stop()

	m.buffer.CloseWriter()
	m.buffer.Flush()
}

func (m *MemoryLogger) Bytes() []byte {
	return m.buffer.Bytes([]byte{})
}

func (m *MemoryLogger) Reader() io.Reader {
	return m.buffer
}

func (m *MemoryLogger) NextLine() string {
	m.scanner.Scan()
	return m.scanner.Text()
}

func (m *MemoryLogger) LineChannel() (chan string, error) {
	if !m.blocking {
		return nil, fmt.Errorf("LineChannel only works with blocking mode")
	}

	c, running := m.getChannel()
	if !running {
		m.start()

		go func() {
			for m.isRunning() {
				line := m.NextLine()
				c <- line
			}

			m.closeChannel()
		}()
	}

	return m.lineChan, nil
}

func (m *MemoryLogger) Sync() error {
	err := m.Logger.Sync()
	if err == nil {
		return m.buffer.Flush()
	}

	return err
}

func (m *MemoryLogger) Reset() {
	m.buffer.Reset()
}

func (m *MemoryLogger) Free() int {
	return m.buffer.Free()
}

func (m *MemoryLogger) Len() int {
	return m.buffer.Length()
}

func (m *MemoryLogger) start() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.running = true
}

func (m *MemoryLogger) isRunning() bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.running
}

func (m *MemoryLogger) stop() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.running = false
}

func (m *MemoryLogger) getChannel() (chan string, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.lineChan != nil {
		return m.lineChan, true
	}

	m.lineChan = make(chan string)
	return m.lineChan, false
}

func (m *MemoryLogger) closeChannel() {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.lineChan != nil {
		close(m.lineChan)
		m.lineChan = nil
	}
}
