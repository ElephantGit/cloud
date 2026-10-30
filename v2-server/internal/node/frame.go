// Package node implements the local Controller side of the Node v1 IPC transport.
// It does not own Workspace admission, persistence, or process supervision.
package node

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const maxFrameLength = 16 * 1024 * 1024

// Message retains the Node envelope and opaque payload, including ACP extension metadata.
// Sequence is absent for commands and present for durable execution events (including zero).
type Message struct {
	Type        string          `json:"message_type"`
	Version     int             `json:"protocol_version"`
	OperationID string          `json:"operation_id,omitempty"`
	ExecutionID string          `json:"execution_id,omitempty"`
	Sequence    *uint64         `json:"sequence,omitempty"`
	Payload     json.RawMessage `json:"payload"`
}

func (m Message) validate() error {
	payload := bytes.TrimSpace(m.Payload)
	if m.Version != 1 || strings.TrimSpace(m.Type) == "" || !json.Valid(payload) || len(payload) == 0 || payload[0] != '{' {
		return errors.New("invalid Node v1 envelope")
	}
	return nil
}

func encode(m Message) ([]byte, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if len(data)+1 > maxFrameLength {
		return nil, errors.New("Node frame too large")
	}
	frame := make([]byte, 5+len(data))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(data)+1))
	frame[4] = 1
	copy(frame[5:], data)
	return frame, nil
}

func readFrame(r io.Reader) (Message, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Message{}, err
	}
	length := binary.BigEndian.Uint32(header[:])
	if length < 1 || length > maxFrameLength {
		return Message{}, fmt.Errorf("invalid Node frame length %d", length)
	}
	var tag [1]byte
	if _, err := io.ReadFull(r, tag[:]); err != nil {
		return Message{}, err
	}
	if tag[0] != 1 {
		return Message{}, errors.New("unsupported Node frame tag")
	}
	data := make([]byte, int(length)-1)
	if _, err := io.ReadFull(r, data); err != nil {
		return Message{}, err
	}
	var message Message
	if err := json.Unmarshal(data, &message); err != nil {
		return Message{}, fmt.Errorf("decode Node envelope: %w", err)
	}
	return message, message.validate()
}

// writeAll handles legal short writes; a failed partial frame requires closing the connection.
func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
