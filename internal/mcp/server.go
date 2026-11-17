// Package mcp is a small Model Context Protocol server over stdio
// (`jedkx-cli _mcp`). Claude Code and Codex start it when `jedkx-cli share`
// added it, so each app can list, read and search the chats of both — in the
// terminal, the desktop apps and the IDE extensions alike.
//
// It only reads: the same adapters as the list, nothing is written anywhere.
// It lives as long as the tool keeps it open and exits with it.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/jedkx/jedkx-cli/internal/app"
	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/config"
	"github.com/jedkx/jedkx-cli/internal/export"
	"github.com/jedkx/jedkx-cli/internal/meta"
	"github.com/jedkx/jedkx-cli/internal/source"
)

const protocolVersion = "2025-06-18"

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Load returns the chats to serve; tests replace it.
var Load = func() []catalog.Item {
	return catalog.Load(meta.Load(meta.File()), config.Hidden(config.Load())...).Items
}

// Serve answers newline-delimited JSON-RPC messages until in is closed.
func Serve(in io.Reader, out io.Writer) error {
	r := bufio.NewReaderSize(in, 1<<20)
	enc := json.NewEncoder(out)
	for {
		line, err := r.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			var req request
			if jerr := json.Unmarshal(line, &req); jerr != nil {
				_ = enc.Encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			} else if resp, ok := handle(req); ok {
				if e := enc.Encode(resp); e != nil {
					return e
				}
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// handle answers one request; notifications (no id) get no answer.
func handle(req request) (response, bool) {
	if len(req.ID) == 0 {
		return response{}, false
	}
	resp := response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := p.ProtocolVersion
		if v == "" {
			v = protocolVersion
		}
		resp.Result = map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": app.Name, "version": app.Version},
			"instructions": "Chats of Claude Code and Codex on this computer. Use list_sessions or search_sessions to find one, " +
				"then read_session to read it and carry the work on here.",
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": tools()}
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = &rpcError{-32602, "invalid params"}
			break
		}
		text, err := call(p.Name, p.Arguments)
		if err != nil {
			resp.Result = toolResult(err.Error(), true)
		} else {
			resp.Result = toolResult(text, false)
		}
	default:
		resp.Error = &rpcError{-32601, "method not found: " + req.Method}
	}
	return resp, true
}

func toolResult(text string, isErr bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isErr,
	}
}

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var (
	propTool    = map[string]any{"type": "string", "enum": []string{"claude", "codex"}, "description": "only chats of this tool"}
	propProject = map[string]any{"type": "string", "description": "only chats whose folder contains this text (name or path)"}
	propLimit   = map[string]any{"type": "integer", "description": "at most this many (default 20)"}
)

func tools() []map[string]any {
	return []map[string]any{
		{
			"name":        "list_sessions",
			"description": "List Claude Code and Codex chats on this computer, newest first: tool, id, time, folder and title.",
			"inputSchema": schema(map[string]any{"tool": propTool, "project": propProject, "limit": propLimit}),
		},
		{
			"name": "search_sessions",
			"description": "Find chats of both tools by words in their title, first prompt, folder or branch, " +
				"and inside their messages. Same syntax as the jedkx-cli list: \"exact words\", -leave-out, tool:codex, since:3d.",
			"inputSchema": schema(map[string]any{
				"query": map[string]any{"type": "string"}, "tool": propTool, "project": propProject, "limit": propLimit,
			}, "query"),
		},
		{
			"name": "read_session",
			"description": "Read one chat (Claude Code or Codex) as Markdown, with its folder and the command that resumes it. " +
				"Use it to continue work started in the other app.",
			"inputSchema": schema(map[string]any{
				"id":   map[string]any{"type": "string", "description": "the chat id (or its first characters) from list_sessions"},
				"tool": propTool,
				"last": map[string]any{"type": "integer", "description": "only the last N messages (default: all that fit)"},
			}, "id"),
		},
	}
}

type args struct {
	Tool    string `json:"tool"`
	Project string `json:"project"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
	ID      string `json:"id"`
	Last    int    `json:"last"`
}

func call(name string, raw json.RawMessage) (string, error) {
	var a args
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &a); err != nil {
			return "", fmt.Errorf("bad arguments: %v", err)
		}
	}
	if a.Limit <= 0 {
		a.Limit = 20
	}
	switch name {
	case "list_sessions":
		return list(filter(Load(), a), a.Limit, nil), nil
	case "search_sessions":
		if strings.TrimSpace(a.Query) == "" {
			return "", fmt.Errorf("query is empty")
		}
		return search(filter(Load(), a), a), nil
	case "read_session":
		return read(Load(), a)
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

func filter(items []catalog.Item, a args) []catalog.Item {
	var out []catalog.Item
	p := catalog.Fold(a.Project)
	for _, it := range items {
		if it.Archived || (a.Tool != "" && it.Tool != a.Tool) {
			continue
		}
		if p != "" && !strings.Contains(catalog.Fold(it.CWD), p) {
			continue
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out
}

func list(items []catalog.Item, limit int, note map[string]string) string {
	if len(items) == 0 {
		return "No chats found."
	}
	var b strings.Builder
	for i, it := range items {
		if i == limit {
			fmt.Fprintf(&b, "… and %d more (raise limit or narrow it down)\n", len(items)-limit)
			break
		}
		fmt.Fprintf(&b, "- [%s] %s  %s  %s\n  id: %s\n", it.Tool, it.Updated.Format("2006-01-02 15:04"), it.CWD, it.Label(), it.ID)
		if n := note[it.Key()]; n != "" {
			fmt.Fprintf(&b, "  %s\n", n)
		}
	}
	return b.String()
}

func search(items []catalog.Item, a args) string {
	q := catalog.Parse(a.Query)
	score := map[string]int{}
	var hits []catalog.Item
	for _, it := range items {
		if !q.Filter(it) {
			continue
		}
		if s, ok, _ := q.Match(it); ok {
			score[it.Key()] = s + 1000 // title / prompt hits first
			hits = append(hits, it)
		}
	}
	inside := catalog.SearchText(items, a.Query)
	note := map[string]string{}
	for _, it := range items {
		s, ok := inside[it.Key()]
		if !ok {
			continue
		}
		if _, seen := score[it.Key()]; !seen {
			hits = append(hits, it)
		}
		score[it.Key()] += s
		note[it.Key()] = "(found in its messages)"
	}
	sort.SliceStable(hits, func(i, j int) bool { return score[hits[i].Key()] > score[hits[j].Key()] })
	return list(hits, a.Limit, note)
}

// maxRead keeps one answer well inside a model's context.
const maxRead = 60000

func read(items []catalog.Item, a args) (string, error) {
	id := strings.TrimSpace(a.ID)
	if id == "" {
		return "", fmt.Errorf("id is empty")
	}
	var found []catalog.Item
	for _, it := range items {
		if a.Tool != "" && it.Tool != a.Tool {
			continue
		}
		if it.ID == id {
			found = []catalog.Item{it}
			break
		}
		if strings.HasPrefix(it.ID, id) {
			found = append(found, it)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no chat with id %q", id)
	case 1:
	default:
		return "", fmt.Errorf("%d chats start with %q — give more of the id", len(found), id)
	}
	it := found[0]
	tr, ok := catalog.Adapter(it.Tool).(source.Transcriber)
	if !ok {
		return "", fmt.Errorf("reading %s chats is not supported", it.Tool)
	}
	turns, err := tr.Transcript(it.Session)
	if err != nil {
		return "", err
	}
	if a.Last > 0 {
		turns = source.KeepLast(turns, a.Last)
	}
	head := fmt.Sprintf("tool: %s\nfolder: %s\nupdated: %s\nresume: %s\n\n", it.Tool, it.CWD, it.Updated.Format(time.RFC3339), resumeHint(it))
	body := export.Markdown(it, turns)
	if len(body) > maxRead { // keep the end: that is where the work stopped
		cut := len(body) - maxRead
		for cut < len(body) && body[cut]&0xC0 == 0x80 {
			cut++
		}
		body = "… (earlier messages left out; ask with last: N for fewer)\n\n" + body[cut:]
	}
	return head + body, nil
}

func resumeHint(it catalog.Item) string {
	if it.Tool == "codex" {
		return "codex resume " + it.ID
	}
	return "claude --resume " + it.ID
}
