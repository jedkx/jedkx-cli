package mcp

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/source"
)

// fakeCodexChat writes a tiny Codex rollout file and returns it as an item.
func fakeCodexChat(t *testing.T) catalog.Item {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rollout-2026-11-17T10-00-00-0000aaaa-bbbb.jsonl")
	lines := []string{
		`{"type":"session_meta","payload":{"id":"0000aaaa-bbbb","cwd":"C:\\.jedkx\\demo"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"fix the login form"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Password check is still missing."}]}}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return catalog.Item{Session: source.Session{
		Tool: "codex", ID: "0000aaaa-bbbb", Title: "login form", CWD: `C:\.jedkx\demo`,
		Updated: time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC), Path: p,
	}}
}

func roundTrip(t *testing.T, msgs ...string) []map[string]any {
	t.Helper()
	var out strings.Builder
	if err := Serve(strings.NewReader(strings.Join(msgs, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var res []map[string]any
	sc := bufio.NewScanner(strings.NewReader(out.String()))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("bad json %q: %v", sc.Text(), err)
		}
		res = append(res, m)
	}
	return res
}

func text(t *testing.T, m map[string]any) string {
	t.Helper()
	r, _ := m["result"].(map[string]any)
	c, _ := r["content"].([]any)
	if len(c) == 0 {
		t.Fatalf("no content in %v", m)
	}
	return c[0].(map[string]any)["text"].(string)
}

func TestHandshakeAndTools(t *testing.T) {
	res := roundTrip(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"nope"}`,
	)
	if len(res) != 3 { // the notification gets no answer
		t.Fatalf("got %d answers", len(res))
	}
	if v := res[0]["result"].(map[string]any)["protocolVersion"]; v != "2025-03-26" {
		t.Fatalf("protocol %v", v)
	}
	tl := res[1]["result"].(map[string]any)["tools"].([]any)
	var names []string
	for _, x := range tl {
		names = append(names, x.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "list_sessions,search_sessions,read_session" {
		t.Fatalf("tools %v", names)
	}
	if res[2]["error"] == nil {
		t.Fatal("unknown method must be an error")
	}
}

func TestListSearchRead(t *testing.T) {
	it := fakeCodexChat(t)
	old := Load
	Load = func() []catalog.Item { return []catalog.Item{it} }
	t.Cleanup(func() { Load = old })

	res := roundTrip(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_sessions","arguments":{"tool":"codex"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_sessions","arguments":{"query":"password"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_session","arguments":{"id":"0000aa"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read_session","arguments":{"id":"zzz"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_sessions","arguments":{"tool":"claude"}}}`,
	)
	if got := text(t, res[0]); !strings.Contains(got, "0000aaaa-bbbb") || !strings.Contains(got, "login form") {
		t.Fatalf("list: %q", got)
	}
	if got := text(t, res[1]); !strings.Contains(got, "0000aaaa-bbbb") || !strings.Contains(got, "found in its messages") {
		t.Fatalf("search: %q", got)
	}
	got := text(t, res[2])
	for _, want := range []string{"codex resume 0000aaaa-bbbb", "fix the login form", "Password check is still missing."} {
		if !strings.Contains(got, want) {
			t.Fatalf("read lacks %q: %q", want, got)
		}
	}
	if r := res[3]["result"].(map[string]any); r["isError"] != true {
		t.Fatalf("unknown id must be a tool error: %v", r)
	}
	if got := text(t, res[4]); got != "No chats found." {
		t.Fatalf("filter: %q", got)
	}
}
