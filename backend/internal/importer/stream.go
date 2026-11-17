package importer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// streamHandler receives the objects of one resource as a source produces them.
type streamHandler struct {
	// SetTotal reports the number of objects when the source knows it.
	SetTotal func(n int)
	// Object consumes one object; a returned error aborts the stream.
	Object func(o lsObject) error
	// Invalid reports an element that could not be decoded; the stream continues.
	Invalid func(err error)
}

func (h *streamHandler) setTotal(n int) {
	if h.SetTotal != nil {
		h.SetTotal(n)
	}
}

func (h *streamHandler) invalid(err error) {
	if h.Invalid != nil {
		h.Invalid(err)
	}
}

// handlerError wraps an error returned by streamHandler.Object so that callers
// can tell it apart from a syntax error in the input.
type handlerError struct{ err error }

func (e *handlerError) Error() string { return e.err.Error() }
func (e *handlerError) Unwrap() error { return e.err }

func (h *streamHandler) emit(raw json.RawMessage) error {
	o, err := parseObject(raw)
	if err != nil {
		h.invalid(err)
		return nil
	}
	if err := h.Object(o); err != nil {
		return &handlerError{err}
	}
	return nil
}

// skipBOM drops a UTF-8 byte order mark.
func skipBOM(r io.Reader) *bufio.Reader {
	br := bufio.NewReaderSize(r, 1<<20)
	if b, err := br.Peek(3); err == nil && bytes.Equal(b, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = br.Discard(3)
	}
	return br
}

// streamJSON reads JSON:API documents ({"data":[...]}), bare arrays of
// resource objects, single resource objects and NDJSON (any number of the
// former concatenated) and emits every resource object it finds.
func streamJSON(r io.Reader, h *streamHandler) error {
	dec := json.NewDecoder(skipBOM(r))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		d, ok := tok.(json.Delim)
		if !ok {
			continue // stray scalar between documents
		}
		switch d {
		case '[':
			err = streamArrayBody(dec, h)
		case '{':
			err = streamTopObject(dec, h)
		}
		if err != nil {
			return err
		}
	}
}

func streamArrayBody(dec *json.Decoder, h *streamHandler) error {
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		if err := h.emit(raw); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil { // ']'
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

// readFields consumes the remainder of an object whose '{' was already read.
func readFields(dec *json.Decoder, visit func(key string) error) error {
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		key, _ := kt.(string)
		if err := visit(key); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil { // '}'
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func skipValue(dec *json.Decoder) error {
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func streamTopObject(dec *json.Decoder, h *streamHandler) error {
	fields := map[string]json.RawMessage{}
	sawData := false
	err := readFields(dec, func(key string) error {
		switch key {
		case "data":
			sawData = true
			tok, err := dec.Token()
			if err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
			d, ok := tok.(json.Delim)
			if !ok {
				return nil // "data": null
			}
			if d == '[' {
				return streamArrayBody(dec, h)
			}
			return streamResourceBody(dec, h)
		case "included", "meta", "jsonapi", "errors":
			return skipValue(dec)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		// "links" is a JSON:API links object, except on flat variant objects
		// where it is the array of variant links.
		if key == "links" && !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
			return nil
		}
		fields[key] = raw
		return nil
	})
	if err != nil {
		return err
	}
	if sawData || len(fields) == 0 {
		return nil
	}
	return emitFields(fields, h)
}

func streamResourceBody(dec *json.Decoder, h *streamHandler) error {
	fields := map[string]json.RawMessage{}
	err := readFields(dec, func(key string) error {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		fields[key] = raw
		return nil
	})
	if err != nil {
		return err
	}
	return emitFields(fields, h)
}

func emitFields(fields map[string]json.RawMessage, h *streamHandler) error {
	o, err := objectFromFields(fields)
	if err != nil {
		h.invalid(err)
		return nil
	}
	if err := h.Object(o); err != nil {
		return &handlerError{err}
	}
	return nil
}
