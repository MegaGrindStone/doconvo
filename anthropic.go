package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type anthropic struct {
	apiKey      string
	model       string
	temperature float64
	client      *http.Client
}

type anthropicChatRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Temperature float64            `json:"temperature"`
	Stream      bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicChatResponse struct {
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Text string `json:"text"`
}

type anthropicStreamResponse struct {
	Type  string `json:"type"`
	Delta struct {
		Text string `json:"text"`
	} `json:"delta"`
}

const (
	anthropicAPIEndpoint = "https://api.anthropic.com/v1"
)

func newAnthropic(apiKey string, model string, temperature float64) anthropic {
	return anthropic{
		apiKey:      apiKey,
		model:       model,
		temperature: temperature,
		client:      &http.Client{},
	}
}

func (a anthropic) chat(ctx context.Context, chats []chat) llmResponse {
	systemChat, cs := extractSystemChat(chats)

	msgs := make([]anthropicMessage, len(cs))
	for i, chat := range cs {
		msgs[i] = anthropicMessage{
			Role:    chat.Role,
			Content: chat.Content,
		}
	}

	reqBody := anthropicChatRequest{
		Model:       a.model,
		Messages:    msgs,
		Temperature: a.temperature,
		Stream:      false,
		System:      systemChat,
		MaxTokens:   a.maxTokens(),
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return llmResponse{
			err: fmt.Errorf("error marshaling request: %w", err),
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIEndpoint+"/messages", bytes.NewBuffer(jsonBody))
	if err != nil {
		return llmResponse{
			err: fmt.Errorf("error creating request: %w", err),
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(req)
	if err != nil {
		return llmResponse{
			err: fmt.Errorf("error sending request: %w", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return llmResponse{
			err: fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body)),
		}
	}

	var response anthropicChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return llmResponse{
			err: fmt.Errorf("error decoding response: %w", err),
		}
	}

	if len(response.Content) == 0 {
		return llmResponse{
			err: fmt.Errorf("empty response content"),
		}
	}

	return llmResponse{
		content: response.Content[0].Text,
	}
}

func (a anthropic) chatStream(ctx context.Context, chats []chat) <-chan llmResponse {
	responseChan := make(chan llmResponse)

	go func() {
		defer close(responseChan)

		systemChat, cs := extractSystemChat(chats)

		msgs := make([]anthropicMessage, len(cs))
		for i, chat := range cs {
			msgs[i] = anthropicMessage{
				Role:    chat.Role,
				Content: chat.Content,
			}
		}

		reqBody := anthropicChatRequest{
			Model:       a.model,
			Messages:    msgs,
			Temperature: a.temperature,
			Stream:      true,
			System:      systemChat,
			MaxTokens:   a.maxTokens(),
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			responseChan <- llmResponse{
				err: fmt.Errorf("error marshaling request: %w", err),
			}
			return
		}

		req, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIEndpoint+"/messages", bytes.NewBuffer(jsonBody))
		if err != nil {
			responseChan <- llmResponse{
				err: fmt.Errorf("error creating request: %w", err),
			}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", a.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := a.client.Do(req)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			responseChan <- llmResponse{
				err: fmt.Errorf("error sending request: %w", err),
			}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			responseChan <- llmResponse{
				err: fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body)),
			}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			var streamResp anthropicStreamResponse
			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				responseChan <- llmResponse{
					err: fmt.Errorf("error decoding response: %w", err),
				}
				return
			}

			if streamResp.Type == "content_block_delta" && streamResp.Delta.Text != "" {
				responseChan <- llmResponse{
					content: streamResp.Delta.Text,
				}
			}

			if streamResp.Type == "message_stop" {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			if !errors.Is(err, context.Canceled) {
				responseChan <- llmResponse{
					err: fmt.Errorf("error reading response: %w", err),
				}
			}
		}
	}()

	return responseChan
}

func (a anthropic) maxTokens() int {
	if strings.HasPrefix(a.model, "claude-3-5-sonnet") ||
		strings.HasPrefix(a.model, "claude-3-5-haiku") {
		return 8192
	}
	return 4096
}
