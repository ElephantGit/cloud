package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Identity pins observations to a persistent Node and its current process incarnation.
type Identity struct {
	NodeID        string `json:"node_id"`
	IncarnationID string `json:"incarnation_id"`
}

// Options are local deployment settings, never user-supplied request fields.
type Options struct {
	SocketPath           string
	ControllerID         string
	ExpectedNodeID       string
	HeartbeatInterval    time.Duration
	FrameTimeout         time.Duration
	RequiredCapabilities []string
}

// Client owns one IPC connection and one heartbeat goroutine. One caller owns Receive;
// Send/Ack/Close may run concurrently. Read/write cancellation closes this low-level connection
// because a partially transferred frame cannot be resumed safely. Reconnect/replay belongs to
// the runtime; cancelling an HTTP observer must never cancel this transport context.
type Client struct {
	conn          net.Conn
	options       Options
	identity      Identity
	writeToken    chan struct{}
	done          chan struct{}
	heartbeatDone chan struct{}
	closeOnce     sync.Once
}

// Dial negotiates Node v1 and required capabilities before starting heartbeats.
// ctx controls connection/handshake only; Close owns the successful client's lifetime.
func Dial(ctx context.Context, opts Options) (*Client, error) {
	if !filepath.IsAbs(opts.SocketPath) || strings.TrimSpace(opts.ControllerID) == "" || strings.TrimSpace(opts.ExpectedNodeID) == "" || opts.HeartbeatInterval <= 0 || opts.FrameTimeout <= opts.HeartbeatInterval {
		return nil, errors.New("invalid Node IPC options")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", opts.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("connect Node IPC: %w", err)
	}
	c := &Client{conn: conn, options: opts, writeToken: make(chan struct{}, 1), done: make(chan struct{}), heartbeatDone: make(chan struct{})}
	c.writeToken <- struct{}{}
	hello, _ := json.Marshal(map[string]any{"controller_id": opts.ControllerID, "supported_versions": []int{1}})
	if err = c.Send(ctx, Message{Type: "hello", Version: 1, Payload: hello}); err == nil {
		var message Message
		message, err = c.read(ctx)
		if err == nil {
			err = c.accept(message)
		}
	}
	if err != nil {
		c.shutdown()
		return nil, fmt.Errorf("Node handshake: %w", err)
	}
	go c.heartbeat()
	return c, nil
}

func (c *Client) accept(message Message) error {
	var accepted struct {
		Version      int      `json:"selected_version"`
		Node         Identity `json:"node"`
		Capabilities []string `json:"capabilities"`
	}
	if message.Type != "hello_accepted" || json.Unmarshal(message.Payload, &accepted) != nil || accepted.Version != 1 || accepted.Node.NodeID != c.options.ExpectedNodeID || strings.TrimSpace(accepted.Node.IncarnationID) == "" {
		return errors.New("Node identity or negotiated version mismatch")
	}
	capabilities := make(map[string]bool)
	for _, capability := range accepted.Capabilities {
		if capability == "" || capabilities[capability] {
			return errors.New("invalid Node capabilities")
		}
		capabilities[capability] = true
	}
	if len(capabilities) == 0 {
		return errors.New("Node advertises no capabilities")
	}
	for _, required := range c.options.RequiredCapabilities {
		if !capabilities[required] {
			return fmt.Errorf("Node missing required capability %q", required)
		}
	}
	c.identity = accepted.Node
	return nil
}

// Identity returns the immutable identity established by Hello.
func (c *Client) Identity() Identity { return c.identity }

// Send serializes commands and heartbeats and bounds every frame write. Successful return
// means bytes were sent, not that Node admitted or completed the command.
func (c *Client) Send(ctx context.Context, message Message) error {
	data, err := encode(message)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return net.ErrClosed
	case <-c.writeToken:
	}
	defer func() { c.writeToken <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.conn.SetWriteDeadline(c.deadline(ctx)); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, c.shutdown)
	err = writeAll(c.conn, data)
	stop()
	if err != nil {
		c.shutdown()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (c *Client) read(ctx context.Context) (Message, error) {
	if err := ctx.Err(); err != nil {
		return Message{}, err
	}
	if err := c.conn.SetReadDeadline(c.deadline(ctx)); err != nil {
		return Message{}, err
	}
	stop := context.AfterFunc(ctx, c.shutdown)
	message, err := readFrame(c.conn)
	stop()
	if err != nil {
		c.shutdown()
	}
	if ctx.Err() != nil {
		return Message{}, ctx.Err()
	}
	return message, err
}

// Receive skips validated heartbeats and returns every other message unchanged. In particular,
// it never ACKs durable events: callers must persist/process them before explicitly calling Ack.
func (c *Client) Receive(ctx context.Context) (Message, error) {
	for {
		message, err := c.read(ctx)
		if err != nil {
			return Message{}, err
		}
		if message.Type != "heartbeat" {
			return message, nil
		}
		var heartbeat struct {
			Node Identity `json:"node"`
		}
		if json.Unmarshal(message.Payload, &heartbeat) != nil || heartbeat.Node != c.identity {
			c.shutdown()
			return Message{}, errors.New("Node heartbeat identity changed")
		}
	}
}

// Ack acknowledges exactly the supplied execution event after its owner has retained it.
func (c *Client) Ack(ctx context.Context, event Message) error {
	if event.ExecutionID == "" || event.OperationID == "" || event.Sequence == nil {
		return errors.New("cannot ACK an event without execution coordinates")
	}
	payload, _ := json.Marshal(map[string]string{"node_id": c.identity.NodeID})
	return c.Send(ctx, Message{Type: "event_ack", Version: 1, OperationID: event.OperationID, ExecutionID: event.ExecutionID, Sequence: event.Sequence, Payload: payload})
}

func (c *Client) deadline(ctx context.Context) time.Time {
	deadline := time.Now().Add(c.options.FrameTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		return d
	}
	return deadline
}

func (c *Client) heartbeat() {
	defer close(c.heartbeatDone)
	ticker := time.NewTicker(c.options.HeartbeatInterval)
	defer ticker.Stop()
	payload, _ := json.Marshal(map[string]string{"controller_id": c.options.ControllerID})
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if err := c.Send(context.Background(), Message{Type: "heartbeat", Version: 1, Payload: payload}); err != nil {
				c.shutdown()
				return
			}
		}
	}
}

func (c *Client) shutdown() {
	c.closeOnce.Do(func() { close(c.done); _ = c.conn.Close() })
}

// Close unblocks pending I/O and joins the heartbeat worker. It is safe to call repeatedly.
func (c *Client) Close() error {
	c.shutdown()
	<-c.heartbeatDone
	return nil
}
