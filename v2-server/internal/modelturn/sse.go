package modelturn

import (
	"bytes"
	"unicode/utf8"
)

// stream handles CR, LF and CRLF even across writes. Each event is dispatched only at its blank
// line boundary; an unterminated final event cannot invent a successful completion.
func (w *responseWriter) stream(data []byte) error {
	for _, b := range data {
		if w.skipLF {
			w.skipLF = false
			if b == '\n' {
				continue
			}
		}
		if b == '\r' || b == '\n' {
			if err := w.consumeLine(); err != nil {
				return err
			}
			w.line = w.line[:0]
			w.skipLF = b == '\r'
			continue
		}
		if len(w.line) == maxEventBytes {
			return ErrLimit
		}
		w.line = append(w.line, b)
	}
	return nil
}

func (w *responseWriter) consumeLine() error {
	if !w.sseStarted {
		w.sseStarted = true
		// The first line is buffered across writes, so even a byte-split UTF-8 BOM is
		// stripped exactly once, only at the beginning of the entire SSE stream.
		w.line = bytes.TrimPrefix(w.line, []byte{0xef, 0xbb, 0xbf})
	}
	if !utf8.Valid(w.line) {
		return ErrInvalidResponse
	}
	if len(w.line) == 0 {
		if !w.hasData {
			return nil
		}
		data := w.event[:len(w.event)-1] // remove the SSE delimiter appended for the last data line
		if w.done {
			return ErrInvalidResponse
		}
		if bytes.Equal(data, []byte("[DONE]")) {
			if !w.finished {
				return ErrIncomplete
			}
			w.done = true
		} else if err := w.consume(data, true); err != nil {
			return err
		}
		w.event, w.hasData = w.event[:0], false
		return nil
	}
	if w.line[0] == ':' {
		return nil
	}
	field, value, found := bytes.Cut(w.line, []byte(":"))
	for _, b := range field {
		if b >= utf8.RuneSelf {
			// In particular, a second/misplaced/damaged BOM must not turn a data
			// field into an ignored field and silently lose part of the answer.
			return ErrInvalidResponse
		}
	}
	if !bytes.Equal(field, []byte("data")) {
		return nil // standard SSE id/event/retry fields do not change completion semantics
	}
	if found && len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	if len(value)+1 > maxEventBytes-len(w.event) {
		return ErrLimit
	}
	w.event = append(w.event, value...)
	w.event = append(w.event, '\n')
	w.hasData = true
	return nil
}
