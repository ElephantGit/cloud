//go:build linux

package node

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func socketDirectory(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "v-ipc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}

func fixture(t *testing.T, serve func(net.Conn) error) (Options, <-chan error) {
	t.Helper()
	path := filepath.Join(socketDirectory(t), "control.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		done <- serve(conn)
	}()
	return Options{SocketPath: path, ControllerID: "controller", ExpectedNodeID: "node", HeartbeatInterval: 20 * time.Millisecond, FrameTimeout: time.Second, RequiredCapabilities: []string{"agent_session"}}, done
}

func greeting(conn net.Conn, nodeID string) error {
	m, err := readFrame(conn)
	if err != nil {
		return err
	}
	if m.Type != "hello" {
		return errors.New("missing hello")
	}
	payload, _ := json.Marshal(map[string]any{"selected_version": 1, "node": Identity{NodeID: nodeID, IncarnationID: "incarnation"}, "capabilities": []string{"agent_session"}})
	frame, _ := encode(Message{Type: "hello_accepted", Version: 1, Payload: payload})
	return writeAll(conn, frame)
}

func TestClientExplicitAckAndHeartbeats(t *testing.T) {
	sequence := uint64(4)
	permitAck := make(chan struct{})
	opts, done := fixture(t, func(conn net.Conn) error {
		if err := greeting(conn, "node"); err != nil {
			return err
		}
		frame, _ := encode(Message{Type: "thread_event", Version: 1, OperationID: "op", ExecutionID: "exec", Sequence: &sequence, Payload: json.RawMessage(`{"record":{"type":"turnEnded"}}`)})
		if err := writeAll(conn, frame); err != nil {
			return err
		}
		// The only automatic message must be a heartbeat, never an ACK before persistence.
		m, err := readFrame(conn)
		if err != nil {
			return err
		}
		if m.Type != "heartbeat" {
			return errors.New("event was acknowledged before owner committed it")
		}
		close(permitAck)
		for {
			m, err = readFrame(conn)
			if err != nil {
				return err
			}
			if m.Type == "heartbeat" {
				continue
			}
			if m.Type != "event_ack" || m.ExecutionID != "exec" || m.OperationID != "op" || m.Sequence == nil || *m.Sequence != 4 {
				return errors.New("wrong ACK coordinates")
			}
			return nil
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	m, err := client.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-permitAck:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := client.Ack(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDialRejectsWrongNode(t *testing.T) {
	opts, done := fixture(t, func(conn net.Conn) error { return greeting(conn, "another-node") })
	client, err := Dial(context.Background(), opts)
	if err == nil {
		client.Close()
		t.Fatal("accepted another node")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCloseUnblocksReceive(t *testing.T) {
	opts, done := fixture(t, func(conn net.Conn) error {
		if err := greeting(conn, "node"); err != nil {
			return err
		}
		for {
			if _, err := readFrame(conn); err != nil {
				return nil
			}
		}
	})
	client, err := Dial(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() { _, err := client.Receive(context.Background()); readDone <- err }()
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("closed receive succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("Receive leaked")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestReceiveRejectsChangedIncarnation(t *testing.T) {
	opts, done := fixture(t, func(conn net.Conn) error {
		if err := greeting(conn, "node"); err != nil {
			return err
		}
		frame, _ := encode(Message{Type: "heartbeat", Version: 1, Payload: json.RawMessage(`{"node":{"node_id":"node","incarnation_id":"another"}}`)})
		return writeAll(conn, frame)
	})
	client, err := Dial(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Receive(context.Background()); err == nil {
		t.Fatal("accepted changed incarnation")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type observedRead struct {
	net.Conn
	started chan struct{}
	once    sync.Once
}

func (c *observedRead) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.once.Do(func() { close(c.started) })
	}
	return n, err
}

func TestCancellationClosesPartialFrame(t *testing.T) {
	opts, done := fixture(t, func(conn net.Conn) error {
		if err := greeting(conn, "node"); err != nil {
			return err
		}
		if err := writeAll(conn, []byte{0, 0}); err != nil {
			return err
		}
		for {
			if _, err := readFrame(conn); err != nil {
				return nil
			}
		}
	})
	// Keep heartbeat out of this deliberately partial inbound frame scenario.
	opts.HeartbeatInterval = time.Second
	opts.FrameTimeout = 2 * time.Second
	client, err := Dial(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	// Do not replace the shared connection while heartbeat could use it: use a separate
	// one-shot client with the same wire reader. Wait until it consumes the partial frame,
	// so cancellation cannot race the fixture's write and only test closing an empty stream.
	started := make(chan struct{})
	reader := &Client{conn: &observedRead{Conn: client.conn, started: started}, options: opts, done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := reader.read(ctx); result <- err }()
	select {
	case <-started:
	case err := <-result:
		t.Fatalf("read ended before consuming partial frame: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("partial frame was not consumed")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled read leaked")
	}
	if _, err := client.Receive(context.Background()); err == nil {
		t.Fatal("partial stream was reused")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
