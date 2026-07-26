// Package sse implements the stateful wire framing shared by HTTP providers
// that consume Server-Sent Events.
package sse

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

const DefaultMaxEventBytes = 4 * 1024 * 1024

var ErrEventTooLarge = errors.New("SSE event exceeds size limit")

type Event struct {
	Name string
	Data string
}

type Options struct {
	MaxEventBytes int
	FlushOnEOF    bool
}

type Decoder struct {
	reader        *bufio.Reader
	maxEventBytes int
	flushOnEOF    bool
	eventName     string
	dataLines     []string
	eventBytes    int
	eof           bool
}

func NewDecoder(reader io.Reader, maxEventBytes int) *Decoder {
	return NewDecoderWithOptions(reader, Options{
		MaxEventBytes: maxEventBytes,
		FlushOnEOF:    true,
	})
}

func NewDecoderWithOptions(reader io.Reader, options Options) *Decoder {
	maxEventBytes := options.MaxEventBytes
	if maxEventBytes <= 0 {
		maxEventBytes = DefaultMaxEventBytes
	}
	return &Decoder{
		reader:        bufio.NewReader(reader),
		maxEventBytes: maxEventBytes,
		flushOnEOF:    options.FlushOnEOF,
	}
}

func (d *Decoder) Next() (Event, error) {
	for {
		if d.eof {
			if d.flushOnEOF {
				if event, ok := d.flush(); ok {
					return event, nil
				}
			} else {
				d.reset()
			}
			return Event{}, io.EOF
		}

		line, eof, err := d.readLine()
		if err != nil {
			return Event{}, err
		}
		if line == "" {
			if event, ok := d.flush(); ok {
				if eof {
					d.eof = true
				}
				return event, nil
			}
		} else if err := d.consume(line); err != nil {
			return Event{}, err
		}
		if eof {
			d.eof = true
		}
	}
}

func (d *Decoder) readLine() (line string, eof bool, err error) {
	var builder strings.Builder
	for {
		value, readErr := d.reader.ReadByte()
		switch {
		case readErr == nil:
			switch value {
			case '\n':
				return builder.String(), false, nil
			case '\r':
				next, nextErr := d.reader.ReadByte()
				if nextErr == nil && next != '\n' {
					if unreadErr := d.reader.UnreadByte(); unreadErr != nil {
						return "", false, unreadErr
					}
				} else if nextErr != nil && !errors.Is(nextErr, io.EOF) {
					return "", false, nextErr
				}
				return builder.String(), errors.Is(nextErr, io.EOF), nil
			default:
				builder.WriteByte(value)
				if d.eventBytes+builder.Len() > d.maxEventBytes {
					return "", false, fmt.Errorf("%w: limit is %d bytes", ErrEventTooLarge, d.maxEventBytes)
				}
			}
		case errors.Is(readErr, io.EOF):
			if builder.Len() == 0 {
				return "", true, nil
			}
			return builder.String(), true, nil
		default:
			return "", false, readErr
		}
	}
}

func (d *Decoder) consume(line string) error {
	d.eventBytes += len(line)
	if d.eventBytes > d.maxEventBytes {
		return fmt.Errorf("%w: limit is %d bytes", ErrEventTooLarge, d.maxEventBytes)
	}
	if strings.HasPrefix(line, ":") {
		return nil
	}

	field, value, found := strings.Cut(line, ":")
	if !found {
		value = ""
	}
	if strings.HasPrefix(value, " ") {
		value = value[1:]
	}
	switch field {
	case "event":
		d.eventName = value
	case "data":
		d.dataLines = append(d.dataLines, value)
	}
	return nil
}

func (d *Decoder) flush() (Event, bool) {
	if d.eventName == "" && len(d.dataLines) == 0 {
		d.reset()
		return Event{}, false
	}
	event := Event{
		Name: d.eventName,
		Data: strings.Join(d.dataLines, "\n"),
	}
	d.reset()
	return event, true
}

func (d *Decoder) reset() {
	d.eventName = ""
	d.dataLines = nil
	d.eventBytes = 0
}
