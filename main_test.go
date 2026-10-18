package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentLoopWithMockServer runs the full loop offline: a fake
// OpenAI-compatible server asks for read_file, then answers in text.
func TestAgentLoopWithMockServer(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret-file.txt")
	os.WriteFile(secret, []byte("what animal is the most disagreeable because it always says neigh?"), 0644)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth header")
		}
		var req ChatRequest
		json.NewDecoder(r.Body).Decode(&req)
		calls++
		switch calls {
		case 1:
			if len(req.Tools) != 3 {
				t.Errorf("want 3 tools sent, got %d", len(req.Tools))
			}
			args, _ := json.Marshal(map[string]string{"path": secret})
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Let me read it.","reasoning_content":"need the file","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":` + string(mustJSON(string(args))) + `}}]}}]}`))
		case 2:
			last := req.Messages[len(req.Messages)-1]
			if last.Role != "tool" || last.ToolCallID != "call_1" || !strings.Contains(last.Content, "neigh") {
				t.Errorf("tool result not sent back: %+v", last)
			}
			if req.Messages[1].ReasoningContent != "need the file" {
				t.Errorf("reasoning_content not echoed back")
			}
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"A horse."}}]}`))
		default:
			t.Errorf("unexpected call %d", calls)
		}
	}))
	defer srv.Close()

	inputs := []string{"solve the riddle in secret-file.txt"}
	getUserMessage := func() (string, bool) {
		if len(inputs) == 0 {
			return "", false
		}
		in := inputs[0]
		inputs = inputs[1:]
		return in, true
	}
	client := &Client{BaseURL: srv.URL, APIKey: "test-key", Model: "mock", HTTP: srv.Client()}
	agent := NewAgent(client, getUserMessage, []ToolDefinition{ReadFileDefinition, ListFilesDefinition, EditFileDefinition})
	if err := agent.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("want 2 model calls, got %d", calls)
	}
}

func mustJSON(s string) []byte { b, _ := json.Marshal(s); return b }

func TestEditFileCreateReplaceAndErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "fizz.js")
	run := func(old, new string) (string, error) {
		in, _ := json.Marshal(EditFileInput{Path: p, OldStr: old, NewStr: new})
		return EditFile(in)
	}
	if _, err := run("", "fizzBuzz(100);"); err != nil {
		t.Fatal(err)
	}
	if out, err := run("100", "15"); err != nil || out != "OK" {
		t.Fatalf("replace: %q %v", out, err)
	}
	if b, _ := os.ReadFile(p); string(b) != "fizzBuzz(15);" {
		t.Fatalf("got %q", b)
	}
	if _, err := run("nope", "x"); err == nil {
		t.Fatal("want old_str not found")
	}
	if _, err := run("same", "same"); err == nil {
		t.Fatal("want invalid input")
	}
}

func TestListFilesMarksDirsAndSkipsGit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.MkdirAll(filepath.Join(dir, "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "pkg", "a.go"), nil, 0644)
	in, _ := json.Marshal(ListFilesInput{Path: dir})
	out, err := ListFiles(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != `["pkg/","pkg/a.go"]` {
		t.Fatalf("got %s", out)
	}
}
