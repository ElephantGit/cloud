package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestIneligibleScheduleRuleReturnsClientError(t *testing.T) {
	w := httptest.NewRecorder()
	scheduleActionError(w, "schedule run", fmt.Errorf("record: %w", store.ErrIneligible), "")
	var body struct {
		Code   string            `json:"code"`
		Params map[string]string `json:"params"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 400 || body.Code != "invalid_execution_request" || body.Params["occurrenceId"] != "" {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
}
