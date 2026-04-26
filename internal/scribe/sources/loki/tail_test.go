package loki

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestTailReceivesEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/loki/api/v1/tail") {
			t.Errorf("path = %q", r.URL.Path)
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")
		msg := map[string]any{
			"streams": []any{
				map[string]any{
					"stream": map[string]string{"validator": "node-1"},
					"values": [][]string{
						{"1714039200000000000", "hello"},
					},
				},
			},
		}
		b, _ := json.Marshal(msg)
		_ = c.Write(r.Context(), websocket.MessageText, b)
		time.Sleep(50 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ch := make(chan TailEntry, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { _ = Tail(ctx, wsURL, `{validator="node-1"}`, time.Now().Add(-time.Minute), ch) }()

	select {
	case e := <-ch:
		if e.Line != "hello" {
			t.Errorf("line = %q", e.Line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no entry received within 2s")
	}
}
