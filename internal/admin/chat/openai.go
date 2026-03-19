package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"callit/internal/config"
)

type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIStreamChoice struct {
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
}

type openAIStreamChunk struct {
	Choices []openAIStreamChoice `json:"choices"`
}

type openAIClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func newOpenAIClient(cfg config.AppConfig) *openAIClient {
	timeout := time.Duration(cfg.AI_TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &openAIClient{
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.AI_BaseURL), "/"),
		apiKey:  strings.TrimSpace(cfg.AI_APIKey),
		model:   strings.TrimSpace(cfg.AI_Model),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *openAIClient) Validate() error {
	if c.baseURL == "" {
		return fmt.Errorf("AI_BASE_URL 不能为空")
	}
	if c.apiKey == "" {
		return fmt.Errorf("AI_API_KEY 不能为空")
	}
	if c.model == "" {
		return fmt.Errorf("AI_MODEL 不能为空")
	}
	return nil
}

func (c *openAIClient) StreamChat(ctx context.Context, messages []aiMessage, onDelta func(string) error) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}

	requestBody := map[string]any{
		"model":    c.model,
		"stream":   true,
		"messages": messages,
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	target := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", "Callit")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return "", fmt.Errorf("AI 请求失败(%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var output strings.Builder

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		for _, choice := range chunk.Choices {
			text := choice.Delta.Content
			if text == "" {
				continue
			}
			output.WriteString(text)
			if onDelta != nil {
				if err := onDelta(text); err != nil {
					return output.String(), err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return output.String(), err
	}
	return output.String(), nil
}
