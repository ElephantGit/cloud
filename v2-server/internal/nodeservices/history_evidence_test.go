//go:build linux

package nodeservices

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/wanglongan587/cloud/v2-server/internal/node"
	"github.com/wanglongan587/cloud/v2-server/internal/nodehistory"
)

func TestRealAgentLongHistoryPostgresAfterRestart(t *testing.T) {
	if os.Getenv("V2_NODE_ASSETS_DIR") == "" || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set V2_NODE_ASSETS_DIR and TEST_DATABASE_URL for real Agent/PostgreSQL evidence")
	}
	runRealAgent(t, strings.Repeat("完整结果🙂 quote=\" newline\n", 24000))
}

// The fixture changes only the model's response. The text must cross the unmodified Agent,
// Node history writer and truncated IPC event before the test is allowed to persist it.
func serveLongFixture(w http.ResponseWriter, r *http.Request, m Model, answer string) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		fault(w, 400, "invalid_completion")
		return
	}
	var req struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if json.Unmarshal(data, &req) != nil {
		fault(w, 400, "invalid_completion")
		return
	}
	if len(req.Messages) == 0 || req.Messages[len(req.Messages)-1].Role != "tool" {
		r.Body = io.NopCloser(bytes.NewReader(data))
		FixtureProvider{}.Complete(w, r, m)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	runes := []rune(answer)
	for len(runes) > 0 {
		n := min(2048, len(runes))
		chunk, _ := json.Marshal(map[string]any{"id": "long-fixture", "object": "chat.completion.chunk", "model": m.ID, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": string(runes[:n])}, "finish_reason": nil}}})
		if _, err := (flushWriter{w}).Write(append(append([]byte("data: "), chunk...), []byte("\n\n")...)); err != nil {
			return
		}
		runes = runes[n:]
	}
	_, _ = (flushWriter{w}).Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
}

// This narrowly scoped SQL harness verifies lossless storage and durable-before-ACK at the
// real Node boundary. It is not the page-facing Session/Turn/admission implementation.
type historyEvidence struct {
	conn               *pgx.Conn
	url, schema, table string
}

func openHistoryEvidence(t *testing.T) *historyEvidence {
	t.Helper()
	ctx := context.Background()
	url := os.Getenv("TEST_DATABASE_URL")
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		conn.Close(ctx)
		t.Fatal(err)
	}
	schema := "v2history_" + hex.EncodeToString(id[:])
	table := pgx.Identifier{schema, "events"}.Sanitize()
	e := &historyEvidence{conn: conn, url: url, schema: schema, table: table}
	t.Cleanup(func() {
		if e.conn != nil {
			_, err := e.conn.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
			if err != nil {
				t.Error(err)
			}
			e.conn.Close(ctx)
		}
	})
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "CREATE TABLE "+table+" (execution text, sequence bigint, source jsonb NOT NULL, snapshot jsonb NOT NULL, result jsonb, PRIMARY KEY(execution,sequence))"); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *historyEvidence) retain(t *testing.T, ctx context.Context, m node.Message, snapshot nodehistory.Snapshot, turn *nodehistory.Turn) {
	t.Helper()
	source, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	view, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var result []byte
	if turn != nil {
		result, err = json.Marshal(turn)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.conn.Exec(ctx, "INSERT INTO "+e.table+" VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING", m.ExecutionID, *m.Sequence, source, view, result); err != nil {
		t.Fatal(err)
	}
}

func (e *historyEvidence) reopenFirst(t *testing.T, ctx context.Context) nodehistory.Turn {
	t.Helper()
	if err := e.conn.Close(ctx); err != nil {
		t.Fatal(err)
	}
	var err error
	e.conn, err = pgx.Connect(ctx, e.url)
	if err != nil {
		t.Fatal(err)
	}
	var data []byte
	if err := e.conn.QueryRow(ctx, "SELECT result FROM "+e.table+" WHERE execution='agent' AND source->'payload'->>'turn_id'='first' AND result IS NOT NULL ORDER BY sequence LIMIT 1").Scan(&data); err != nil {
		t.Fatal(err)
	}
	var result nodehistory.Turn
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
