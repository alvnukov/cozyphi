package util

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"iter"
	"sync"
)

// SSE constants used by the event-stream parser.
const (
	ContentEventStream = "text/event-stream"
	sseDataPrefix      = "data:"
	MaxSSETokenSize    = 10 * 1024 * 1024
	SSEBufferSize      = 64 * 1024
)

var sseBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, SSEBufferSize)
		return &buf
	},
}

// ParseDataStream yields SSE data: payloads from an event-stream body.
func ParseDataStream(body io.Reader) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		scanner := bufio.NewScanner(body)
		bufPtr := sseBufferPool.Get().(*[]byte)
		defer func() {
			*bufPtr = (*bufPtr)[:0]
			sseBufferPool.Put(bufPtr)
		}()
		scanner.Buffer(*bufPtr, MaxSSETokenSize)

		prefix := []byte(sseDataPrefix)
		for scanner.Scan() {
			line := scanner.Bytes()
			if !bytes.HasPrefix(line, prefix) {
				continue
			}
			data := line[len(prefix):]
			if len(data) > 0 && data[0] == ' ' {
				data = data[1:]
			}
			if !yield(data, nil) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			yield(nil, fmt.Errorf("SSE stream error: %w", err))
		}
	}
}
