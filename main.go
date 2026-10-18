// aileena-code: a tiny code-editing agent, built step by step after Thorsten
// Ball's "How to Build an Agent" (https://ampcode.com/how-to-build-an-agent).
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	client := NewClient()

	scanner := bufio.NewScanner(os.Stdin)
	getUserMessage := func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		return scanner.Text(), true
	}

	tools := []ToolDefinition{ReadFileDefinition, ListFilesDefinition}
	agent := NewAgent(client, getUserMessage, tools)
	err := agent.Run(context.TODO())
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
	}
}

func NewAgent(client *Client, getUserMessage func() (string, bool), tools []ToolDefinition) *Agent {
	return &Agent{
		client:         client,
		getUserMessage: getUserMessage,
		tools:          tools,
	}
}

type Agent struct {
	client         *Client
	getUserMessage func() (string, bool)
	tools          []ToolDefinition
}

func (a *Agent) Run(ctx context.Context) error {
	conversation := []Message{}

	fmt.Printf("Chat with %s (use 'ctrl-c' to quit)\n", a.client.Model)

	readUserInput := true
	for {
		if readUserInput {
			fmt.Print("\u001b[94mYou\u001b[0m: ")
			userInput, ok := a.getUserMessage()
			if !ok {
				break
			}
			conversation = append(conversation, Message{Role: "user", Content: userInput})
		}

		message, err := a.runInference(ctx, conversation)
		if err != nil {
			return err
		}
		conversation = append(conversation, *message)

		if message.Content != "" {
			fmt.Printf("\u001b[93mAileena\u001b[0m: %s\n", message.Content)
		}
		toolResults := []Message{}
		for _, call := range message.ToolCalls {
			toolResults = append(toolResults, a.executeTool(call.ID, call.Function.Name, call.Function.Arguments))
		}
		if len(toolResults) == 0 {
			readUserInput = true
			continue
		}
		readUserInput = false
		conversation = append(conversation, toolResults...)
	}

	return nil
}

func (a *Agent) executeTool(id, name, arguments string) Message {
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	for _, tool := range a.tools {
		if tool.Name == name {
			fmt.Printf("\u001b[92mtool\u001b[0m: %s(%s)\n", name, arguments)
			response, err := tool.Function(json.RawMessage(arguments))
			if err != nil {
				return Message{Role: "tool", ToolCallID: id, Content: "error: " + err.Error()}
			}
			return Message{Role: "tool", ToolCallID: id, Content: response}
		}
	}
	return Message{Role: "tool", ToolCallID: id, Content: "error: tool not found"}
}

func (a *Agent) runInference(ctx context.Context, conversation []Message) (*Message, error) {
	tools := []Tool{}
	for _, tool := range a.tools {
		tools = append(tools, Tool{Type: "function", Function: ToolFunction{tool.Name, tool.Description, tool.InputSchema}})
	}

	return a.client.Chat(ctx, ChatRequest{
		Model:     a.client.Model,
		MaxTokens: 8192,
		Messages:  conversation,
		Tools:     tools,
	})
}

// --- tools: a name, a description, an input JSON schema, and a Go function.

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema map[string]any
	Function    func(input json.RawMessage) (string, error)
}

// schema hand-writes what the article generates with invopop/jsonschema (keeps us dependency-free).
func schema(properties map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func prop(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

// --- read_file -------------------------------------------------------------

var ReadFileDefinition = ToolDefinition{
	Name:        "read_file",
	Description: "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.",
	InputSchema: schema(map[string]any{
		"path": prop("The relative path of a file in the working directory."),
	}, "path"),
	Function: ReadFile,
}

type ReadFileInput struct {
	Path string `json:"path"`
}

func ReadFile(input json.RawMessage) (string, error) {
	readFileInput := ReadFileInput{}
	if err := json.Unmarshal(input, &readFileInput); err != nil {
		return "", err
	}

	content, err := os.ReadFile(readFileInput.Path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// --- list_files ------------------------------------------------------------

var ListFilesDefinition = ToolDefinition{
	Name:        "list_files",
	Description: "List files and directories at a given path. If no path is provided, lists files in the current directory.",
	InputSchema: schema(map[string]any{
		"path": prop("Optional relative path to list files from. Defaults to current directory if not provided."),
	}),
	Function: ListFiles,
}

type ListFilesInput struct {
	Path string `json:"path,omitempty"`
}

func ListFiles(input json.RawMessage) (string, error) {
	listFilesInput := ListFilesInput{}
	if err := json.Unmarshal(input, &listFilesInput); err != nil {
		return "", err
	}

	dir := "."
	if listFilesInput.Path != "" {
		dir = listFilesInput.Path
	}
	files := []string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir // not in the article: .git floods the context
		}
		if relPath != "." {
			if info.IsDir() {
				files = append(files, relPath+"/")
			} else {
				files = append(files, relPath)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// --- client: the article uses Anthropic's Go SDK; we speak OpenAI-compatible
// /chat/completions with net/http (zero deps; DeepSeek, OpenAI, vLLM, Ollama).

type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

func NewClient() *Client {
	return &Client{
		BaseURL: getenv("AGENT_BASE_URL", "https://api.deepseek.com"),
		APIKey:  getenv("AGENT_API_KEY", os.Getenv("DEEPSEEK_API_KEY")),
		Model:   getenv("AGENT_MODEL", "deepseek-flash"),
		HTTP:    http.DefaultClient,
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Message is one conversation entry. reasoning_content (DeepSeek thinking mode) must be echoed back.
type Message struct {
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

// ToolCall is the model winking: "please run this tool with these arguments".
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ChatRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens,omitempty"`
	Messages  []Message `json:"messages"`
	Tools     []Tool    `json:"tools,omitempty"`
}

func (c *Client) Chat(ctx context.Context, req ChatRequest) (*Message, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, data)
	}

	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("API returned no choices")
	}
	return &out.Choices[0].Message, nil
}
