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

	tools := []ToolDefinition{}
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
