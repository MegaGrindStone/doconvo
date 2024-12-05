package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

type ollama struct {
	host  string
	model string

	client *http.Client
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Model   string            `json:"model"`
	Message ollamaChatMessage `json:"message"`
	Done    bool              `json:"done"`
}

const (
	defaultOllamaHost = "http://127.0.0.1:11434"
)

func newOllama() *ollama {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = defaultOllamaHost
	}
	return &ollama{
		host:   host,
		client: &http.Client{},
		model:  "llama3.2",
	}
}

func (o *ollama) chat(ctx context.Context, chats []chat) aiResponse {
	msgs := make([]ollamaChatMessage, len(chats))
	for i, chat := range chats {
		msgs[i] = ollamaChatMessage{
			Role:    chat.Role,
			Content: chat.Content,
		}
	}

	reqBody := ollamaChatRequest{
		Model:    o.model,
		Messages: msgs,
		Stream:   false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return aiResponse{
			err: fmt.Errorf("error marshaling request: %w", err),
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.host+"/api/chat", bytes.NewBuffer(jsonBody))
	if err != nil {
		return aiResponse{
			err: fmt.Errorf("error creating request: %w", err),
		}
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return aiResponse{
			err: fmt.Errorf("error sending request: %w", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return aiResponse{
			err: fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body)),
		}
	}

	var response ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return aiResponse{
			err: fmt.Errorf("error decoding response: %w", err),
		}
	}

	return aiResponse{
		content: response.Message.Content,
	}
}

func (o *ollama) chatStream(ctx context.Context, chats []chat) <-chan aiResponse {
	responseChan := make(chan aiResponse)

	go func() {
		defer close(responseChan)

		msgs := make([]ollamaChatMessage, len(chats))
		for i, chat := range chats {
			msgs[i] = ollamaChatMessage{
				Role:    chat.Role,
				Content: chat.Content,
			}
		}

		reqBody := ollamaChatRequest{
			Model:    o.model,
			Messages: msgs,
			Stream:   true,
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			responseChan <- aiResponse{
				err: fmt.Errorf("error marshaling request: %w", err),
			}
			return
		}

		req, err := http.NewRequestWithContext(ctx, "POST", o.host+"/api/chat", bytes.NewBuffer(jsonBody))
		if err != nil {
			responseChan <- aiResponse{
				err: fmt.Errorf("error creating request: %w", err),
			}
			return
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := o.client.Do(req)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			responseChan <- aiResponse{
				err: fmt.Errorf("error sending request: %w", err),
			}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			responseChan <- aiResponse{
				err: fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body)),
			}
			return
		}

		decoder := json.NewDecoder(resp.Body)
		for {
			var streamResp ollamaChatResponse
			if err := decoder.Decode(&streamResp); err != nil {
				if err == io.EOF {
					return
				}
				if errors.Is(err, context.Canceled) {
					return
				}
				responseChan <- aiResponse{
					err: fmt.Errorf("error decoding response: %w", err),
				}
				return
			}

			responseChan <- aiResponse{
				content: streamResp.Message.Content,
			}

			if streamResp.Done {
				return
			}
		}
	}()

	return responseChan
}
