package transport

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
)

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	Event string
	Data  string
	ID    string
}

// SSEDecoder decodes Server-Sent Events from a reader
type SSEDecoder struct {
	reader *bufio.Reader
}

// NewSSEDecoder creates a new SSE decoder
func NewSSEDecoder(r io.Reader) *SSEDecoder {
	return &SSEDecoder{
		reader: bufio.NewReader(r),
	}
}

// Decode reads and decodes the next SSE event
func (d *SSEDecoder) Decode() (*SSEEvent, error) {
	event := &SSEEvent{}
	var buffer bytes.Buffer

	for {
		line, err := d.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				if buffer.Len() == 0 {
					return nil, io.EOF
				}
				break
			}
			return nil, fmt.Errorf("error reading SSE line: %w", err)
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if buffer.Len() > 0 {
				break
			}
			continue
		}

		parts := bytes.SplitN(line, []byte(":"), 2)
		if len(parts) != 2 {
			continue
		}

		field := string(bytes.TrimSpace(parts[0]))
		value := string(bytes.TrimSpace(parts[1]))

		switch field {
		case "event":
			event.Event = value
		case "data":
			if buffer.Len() > 0 {
				buffer.WriteByte('\n')
			}
			buffer.WriteString(value)
		case "id":
			event.ID = value
		}
	}

	event.Data = strings.TrimSpace(buffer.String())
	return event, nil
}
