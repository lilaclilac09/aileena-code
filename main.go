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

	agent := NewAgent(client, getUserMessage)
	err := agent.Run(context.TODO())
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
	}
}

func NewAgent(client *Client, getUserMessage func() (string, bool)) *Agent {
	return &Agent{
		client:         client,
		getUserMessage: getUserMessage,
	}
}

type Agent struct {
	client         *Client
	getUserMessage func() (string, bool)
}

func (a *Agent) Run(ctx context.Context) error {
	conversation := []Message{}

	fmt.Printf("Chat with %s (use 'ctrl-c' to quit)\n", a.client.Model)

	for {
		fmt.Print("\u001b[94mYou\u001b[0m: ")
		userInput, ok := a.getUserMessage()
		if !ok {
			break
		}

		conversation = append(conversation, Message{Role: "user", Content: userInput})

		message, err := a.runInference(ctx, conversation)
		if err != nil {
			return err
		}
		conversation = append(conversation, *message)

		if message.Content != "" {
			fmt.Printf("\u001b[93mAileena\u001b[0m: %s\n", message.Content)
		}
	}

	return nil
}

func (a *Agent) runInference(ctx context.Context, conversation []Message) (*Message, error) {
	return a.client.Chat(ctx, ChatRequest{
		Model:     a.client.Model,
		MaxTokens: 8192,
		Messages:  conversation,
	})
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
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type ChatRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens,omitempty"`
	Messages  []Message `json:"messages"`
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
