// Command smoke runs an end-to-end test against a running backend:
// register -> create session -> WS subscribe -> DM sarah -> streamed reply ->
// wait for a surprise task + deadline expiry warning -> end -> evaluation.
//
// Start the server with short timings first, e.g.
//
//	RESPONSE_TIMEOUT=30s TASK_FIRST_DELAY=20s TASK_MIN_INTERVAL=10m TASK_MAX_INTERVAL=10m go run ./cmd/api
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
)

var base = flag.String("base", "http://localhost:8080", "backend base URL")

type env struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func must(err error, what string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", what, err)
		os.Exit(1)
	}
}

func call(method, path, token string, body any, out any) int {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, *base+"/api"+path, r)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	must(err, method+" "+path)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if out != nil {
		_ = json.Unmarshal(b, out)
	}
	fmt.Printf("%s %s -> %d %s\n", method, path, resp.StatusCode, trunc(string(b), 200))
	return resp.StatusCode
}

func trunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func main() {
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	call("GET", "/health", "", nil, nil)
	name := fmt.Sprintf("smoke_%d", time.Now().Unix()%1000000)
	var auth struct {
		Token string                    `json:"token"`
		User  struct{ Username string } `json:"user"`
	}
	if call("POST", "/auth/register", "", map[string]string{"username": name, "email": name + "@example.com", "password": "password123"}, &auth) != 201 {
		os.Exit(1)
	}
	tok := auth.Token
	call("GET", "/me", tok, nil, nil)
	call("GET", "/personas", tok, nil, nil)

	var sess struct{ ID string }
	if call("POST", "/sessions", tok, map[string]any{}, &sess) != 201 {
		os.Exit(1)
	}
	if code := call("POST", "/sessions", tok, map[string]any{}, nil); code != 409 {
		fmt.Println("FAIL expected 409 for second active session")
		os.Exit(1)
	}

	wsURL := strings.Replace(*base, "http", "ws", 1) + "/ws?token=" + tok
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	must(err, "ws dial")
	defer conn.Close(websocket.StatusNormalClosure, "")
	sub, _ := json.Marshal(map[string]any{"type": "subscribe", "data": map[string]string{"sessionId": sess.ID}})
	must(conn.Write(ctx, websocket.MessageText, sub), "ws subscribe")
	ping, _ := json.Marshal(map[string]any{"type": "ping", "data": map[string]any{}})
	must(conn.Write(ctx, websocket.MessageText, ping), "ws ping")

	events := make(chan env, 1000)
	go func() {
		for {
			_, b, err := conn.Read(ctx)
			if err != nil {
				close(events)
				return
			}
			var e env
			if json.Unmarshal(b, &e) == nil {
				events <- e
			}
		}
	}()

	var detail struct {
		Channels []struct {
			ID, Kind, Name string
		} `json:"channels"`
	}
	call("GET", "/sessions/"+sess.ID, tok, nil, &detail)
	var dmSarah string
	for _, c := range detail.Channels {
		if c.Kind == "DM" && c.Name == "sarah" {
			dmSarah = c.ID
		}
	}
	if len(detail.Channels) != 7 || dmSarah == "" {
		fmt.Println("FAIL expected 7 channels incl. DM sarah")
		os.Exit(1)
	}

	time.Sleep(2 * time.Second) // let greeting land
	call("POST", "/sessions/"+sess.ID+"/channels/"+dmSarah+"/messages", tok,
		map[string]string{"content": "Hi Sarah, I am starting on the login page story today. I think I can finish it by Thursday."}, nil)

	seen := map[string]int{}
	var streamed strings.Builder
	gotDone, gotTask, gotExpired, gotWarning := false, false, false, false
	deadline := time.After(4 * time.Minute)
loop:
	for {
		select {
		case e, ok := <-events:
			if !ok {
				fmt.Println("FAIL ws closed")
				os.Exit(1)
			}
			seen[e.Type]++
			switch e.Type {
			case "message.new":
				var m struct {
					AuthorType, Content, PersonaID string
					Streaming                      *bool
				}
				_ = json.Unmarshal(e.Data, &m)
				fmt.Printf("WS message.new  author=%s persona=%s streaming=%v content=%q\n", m.AuthorType, m.PersonaID, m.Streaming != nil && *m.Streaming, trunc(m.Content, 120))
			case "message.delta":
				var d struct{ Delta string }
				_ = json.Unmarshal(e.Data, &d)
				streamed.WriteString(d.Delta)
			case "message.done":
				var m struct{ Content, PersonaID string }
				_ = json.Unmarshal(e.Data, &m)
				fmt.Printf("WS message.done persona=%s content=%q\n", m.PersonaID, trunc(m.Content, 300))
				if m.PersonaID == "sarah" && streamed.Len() > 0 {
					gotDone = true
				}
			case "task.new", "deadline.new", "deadline.updated", "warning.new", "session.updated", "typing", "pong", "error", "task.updated":
				fmt.Printf("WS %s %s\n", e.Type, trunc(string(e.Data), 250))
				if e.Type == "task.new" {
					gotTask = true
				}
				if e.Type == "deadline.updated" && strings.Contains(string(e.Data), "EXPIRED") {
					gotExpired = true
				}
				if e.Type == "warning.new" {
					gotWarning = true
				}
			}
			if gotDone && gotTask && gotExpired && gotWarning {
				break loop
			}
		case <-deadline:
			fmt.Println("TIMEOUT waiting for events")
			break loop
		}
	}
	fmt.Printf("\nstreamed reply OK=%v task OK=%v expiry OK=%v warning OK=%v events=%v\n", gotDone, gotTask, gotExpired, gotWarning, seen)

	call("POST", "/sessions/"+sess.ID+"/end", tok, nil, nil)
	var ev struct {
		Status  string
		Overall int
	}
	for i := 0; i < 60; i++ {
		call("GET", "/sessions/"+sess.ID+"/evaluation", tok, nil, &ev)
		if ev.Status == "DONE" || ev.Status == "FAILED" {
			break
		}
		time.Sleep(5 * time.Second)
	}
	fmt.Printf("evaluation status=%s overall=%d\n", ev.Status, ev.Overall)
	if !(gotDone && gotTask && gotExpired && gotWarning && ev.Status == "DONE") {
		fmt.Println("SMOKE FAILED")
		os.Exit(1)
	}
	fmt.Println("SMOKE PASSED")
}
