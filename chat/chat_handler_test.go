package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/data"
)

func TestHandlerJSONFlowUsesBackend(t *testing.T) {
	reset := setBackendOverride(&fakeBackend{resp: "handler answer"})
	defer reset()

	data.ClearIndex()

	reqBody := `{"prompt":"hello","context":[{"prompt":"old","answer":"resp"}],"topic":"Tech"}`
	r := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(reqBody))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	Handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	ans, ok := resp["answer"].(string)
	if !ok {
		t.Fatalf("answer not a string: %#v", resp["answer"])
	}
	if !strings.Contains(ans, "handler answer") {
		t.Fatalf("answer missing backend text: %s", ans)
	}
}
