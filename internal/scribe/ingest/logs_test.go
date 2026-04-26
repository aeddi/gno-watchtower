package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/coder/websocket"
)

func TestLogsLaneEmitsObservation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		defer c.Close(websocket.StatusNormalClosure, "")
		msg, _ := json.Marshal(map[string]any{
			"streams": []any{map[string]any{
				"stream": map[string]string{"validator": "node-1"},
				"values": [][]string{{"1714039200000000000", "hello"}},
			}},
		})
		_ = c.Write(r.Context(), websocket.MessageText, msg)
		time.Sleep(50 * time.Millisecond)
	}))
	defer srv.Close()

	out := make(chan normalizer.Observation, 4)
	lane := NewLogsLane(strings.Replace(srv.URL, "http", "ws", 1), []string{`{validator="node-1"}`}, 5*time.Second, out)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go lane.Run(ctx)

	select {
	case o := <-out:
		if o.LogEntry == nil || o.LogEntry.Line != "hello" {
			t.Errorf("got %+v", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no observation")
	}
}
