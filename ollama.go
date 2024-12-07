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
	"sync"

	"github.com/philippgille/chromem-go"
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

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
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

func newOllawaWithModel(model string) *ollama {
	o := newOllama()
	o.model = model
	return o
}

func (o *ollama) chat(ctx context.Context, chats []chat) llmResponse {
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
		return llmResponse{
			err: fmt.Errorf("error marshaling request: %w", err),
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.host+"/api/chat", bytes.NewBuffer(jsonBody))
	if err != nil {
		return llmResponse{
			err: fmt.Errorf("error creating request: %w", err),
		}
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
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

	var response ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return llmResponse{
			err: fmt.Errorf("error decoding response: %w", err),
		}
	}

	return llmResponse{
		content: response.Message.Content,
	}
}

func (o *ollama) chatStream(ctx context.Context, chats []chat) <-chan llmResponse {
	responseChan := make(chan llmResponse)

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
			responseChan <- llmResponse{
				err: fmt.Errorf("error marshaling request: %w", err),
			}
			return
		}

		req, err := http.NewRequestWithContext(ctx, "POST", o.host+"/api/chat", bytes.NewBuffer(jsonBody))
		if err != nil {
			responseChan <- llmResponse{
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
				responseChan <- llmResponse{
					err: fmt.Errorf("error decoding response: %w", err),
				}
				return
			}

			responseChan <- llmResponse{
				content: streamResp.Message.Content,
			}

			if streamResp.Done {
				return
			}
		}
	}()

	return responseChan
}

// embeddingFunc returns an EmbeddingFunc that uses the OpenAI API to generate
// embeddings for the given text.
//
// This codes is taken directly from the chromem-go library, with a little modification,
// to make it work to the newer ollama embedding API.
func (c *ollama) embeddingFunc() chromem.EmbeddingFunc {
	var checkedNormalized bool
	checkNormalized := sync.Once{}

	return func(ctx context.Context, text string) ([]float32, error) {
		// Prepare the request body.
		reqBody, err := json.Marshal(map[string]string{
			"model": c.model,
			"input": text, // new ollama API uses "input" instead of "prompt"
		})
		if err != nil {
			return nil, fmt.Errorf("couldn't marshal request body: %w", err)
		}

		// Create the request. Creating it with context is important for a timeout
		// to be possible, because the client is configured without a timeout.
		// Newer ollama API uses /embed instead of /embeddings.
		req, err := http.NewRequestWithContext(ctx, "POST", c.host+"/api/embed", bytes.NewBuffer(reqBody))
		if err != nil {
			return nil, fmt.Errorf("couldn't create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// Send the request.
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("couldn't send request: %w", err)
		}
		defer resp.Body.Close()

		// Check the response status.
		if resp.StatusCode != http.StatusOK {
			return nil, errors.New("error response from the embedding API: " + resp.Status)
		}

		// Read and decode the response body.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("couldn't read response body: %w", err)
		}
		var embeddingResponse ollamaEmbedResponse
		err = json.Unmarshal(body, &embeddingResponse)
		if err != nil {
			return nil, fmt.Errorf("couldn't unmarshal response body: %w", err)
		}

		// Check if the response contains embeddings.
		if len(embeddingResponse.Embeddings) == 0 {
			return nil, errors.New("no embeddings found in the response")
		}
		// In the newer ollama API, the request can take multiple inputs, and the response can contain multiple embeddings.
		// We only want the first embedding, so we take the first element of the array.
		if len(embeddingResponse.Embeddings[0]) == 0 {
			return nil, errors.New("no embeddings found in the response")
		}

		v := embeddingResponse.Embeddings[0]
		checkNormalized.Do(func() {
			if isNormalized(v) {
				checkedNormalized = true
			} else {
				checkedNormalized = false
			}
		})
		if !checkedNormalized {
			v = normalizeVector(v)
		}

		return v, nil
	}
}
