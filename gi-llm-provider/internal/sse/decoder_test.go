package sse

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestDecoderHandlesAllLineEndingsAndEventFields(t *testing.T) {
	for _, separator := range []string{"\n", "\r\n", "\r"} {
		t.Run(strings.ReplaceAll(separator, "\r", "CR"), func(t *testing.T) {
			body := strings.Join([]string{
				": heartbeat",
				"event: message",
				"ignored: value",
				"data: {\"text\":",
				"data: \"hello\"}",
				"",
				"event: trailing",
				"data: final",
			}, separator)
			events, err := decodeAll(strings.NewReader(body), DefaultMaxEventBytes)
			if err != nil {
				t.Fatal(err)
			}
			want := []Event{
				{Name: "message", Data: "{\"text\":\n\"hello\"}"},
				{Name: "trailing", Data: "final"},
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatalf("events = %#v, want %#v", events, want)
			}
		})
	}
}

func TestDecoderPreservesFieldValuesAndFlushesEventWithoutData(t *testing.T) {
	events, err := decodeAll(
		strings.NewReader("event: notice  \ndata:first \ndata:  second\n\n"+
			"event: empty\n\n"),
		DefaultMaxEventBytes,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []Event{
		{Name: "notice  ", Data: "first \n second"},
		{Name: "empty", Data: ""},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestDecoderRejectsOversizedEvent(t *testing.T) {
	decoder := NewDecoder(strings.NewReader("data: 123456\n\n"), 8)
	if _, err := decoder.Next(); !errors.Is(err, ErrEventTooLarge) {
		t.Fatalf("error = %v, want ErrEventTooLarge", err)
	}
}

func TestDecoderCanRequireAnEventDelimiterBeforeEOF(t *testing.T) {
	decoder := NewDecoderWithOptions(
		strings.NewReader("data: incomplete"),
		Options{MaxEventBytes: DefaultMaxEventBytes, FlushOnEOF: false},
	)
	if _, err := decoder.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("error = %v, want EOF", err)
	}

	decoder = NewDecoderWithOptions(
		strings.NewReader("data: complete\n\n"),
		Options{MaxEventBytes: DefaultMaxEventBytes, FlushOnEOF: false},
	)
	event, err := decoder.Next()
	if err != nil {
		t.Fatal(err)
	}
	if event.Data != "complete" {
		t.Fatalf("event = %#v", event)
	}
}

func TestDecoderIsIndependentOfReaderChunkBoundaries(t *testing.T) {
	body := []byte("event: text\rdata: {\"value\":\"café\"}\r\rdata: [DONE]\r\r")
	want, err := decodeAll(strings.NewReader(string(body)), DefaultMaxEventBytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{1, 2, 3, 5, 8, 13} {
		t.Run(fmt.Sprintf("chunk-%d", size), func(t *testing.T) {
			got, err := decodeAll(&fixedChunkReader{data: body, size: size}, DefaultMaxEventBytes)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("events = %#v, want %#v", got, want)
			}
		})
	}
}

func FuzzDecoderChunkBoundaries(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("data: hello\n\n"),
		[]byte("event: text\rdata: café\r\r"),
		[]byte(": comment\r\ndata: one\r\ndata: two\r\n\r\n"),
		[]byte("data: trailing"),
		[]byte("data: \xff\n\n"),
	} {
		f.Add(seed, []byte{1, 2, 3, 5, 8}, true)
		f.Add(seed, []byte{1}, false)
	}

	f.Fuzz(func(t *testing.T, body, pattern []byte, flushOnEOF bool) {
		const maxFuzzBodyBytes = 64 * 1024
		if len(body) > maxFuzzBodyBytes {
			t.Skip()
		}
		if len(pattern) > 64 {
			pattern = pattern[:64]
		}

		want, wantErr := decodeAllWithOptions(
			bytes.NewReader(body),
			Options{
				MaxEventBytes: maxFuzzBodyBytes,
				FlushOnEOF:    flushOnEOF,
			},
		)
		got, gotErr := decodeAllWithOptions(
			&patternChunkReader{data: append([]byte(nil), body...), pattern: pattern},
			Options{
				MaxEventBytes: maxFuzzBodyBytes,
				FlushOnEOF:    flushOnEOF,
			},
		)
		if (gotErr == nil) != (wantErr == nil) {
			t.Fatalf("chunked error = %v, contiguous error = %v", gotErr, wantErr)
		}
		if gotErr != nil && gotErr.Error() != wantErr.Error() {
			t.Fatalf("chunked error = %v, contiguous error = %v", gotErr, wantErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("chunked events = %#v, contiguous events = %#v", got, want)
		}
	})
}

func decodeAll(reader io.Reader, limit int) ([]Event, error) {
	return decodeAllWithOptions(reader, Options{
		MaxEventBytes: limit,
		FlushOnEOF:    true,
	})
}

func decodeAllWithOptions(reader io.Reader, options Options) ([]Event, error) {
	decoder := NewDecoderWithOptions(reader, options)
	var events []Event
	for {
		event, err := decoder.Next()
		if errors.Is(err, io.EOF) {
			return events, nil
		}
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
}

type fixedChunkReader struct {
	data []byte
	size int
}

type patternChunkReader struct {
	data    []byte
	pattern []byte
	next    int
}

func (r *patternChunkReader) Read(target []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	size := 1
	if len(r.pattern) > 0 {
		size = int(r.pattern[r.next%len(r.pattern)])%31 + 1
		r.next++
	}
	size = min(size, len(r.data), len(target))
	copy(target, r.data[:size])
	r.data = r.data[size:]
	return size, nil
}

func (r *fixedChunkReader) Read(target []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	size := min(len(r.data), r.size, len(target))
	copy(target, r.data[:size])
	r.data = r.data[size:]
	return size, nil
}
