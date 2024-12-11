package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"

	"github.com/charmbracelet/huh"
	"github.com/ollama/ollama/api"
	"github.com/philippgille/chromem-go"
	bolt "go.etcd.io/bbolt"
)

type ollamaProvider struct {
	Host string `json:"host"`
}

type ollama struct {
	host        string
	model       string
	temperature float64

	client *api.Client
}

const (
	defaultOllamaHost = "http://127.0.0.1:11434"
)

func (o ollama) chat(ctx context.Context, chats []chat) llmResponse {
	msgs := make([]api.Message, len(chats))
	for i, chat := range chats {
		msgs[i] = api.Message{
			Role:    chat.Role,
			Content: chat.Content,
		}
	}

	f := false
	req := api.ChatRequest{
		Model:    o.model,
		Messages: msgs,
		Stream:   &f,
		Options: map[string]interface{}{
			"temperature": o.temperature,
		},
	}

	var llmResp llmResponse

	if err := o.client.Chat(ctx, &req, func(res api.ChatResponse) error {
		llmResp.content = res.Message.Content
		return nil
	}); err != nil {
		return llmResponse{
			err: fmt.Errorf("error sending request: %w", err),
		}
	}

	return llmResp
}

func (o ollama) chatStream(ctx context.Context, chats []chat) <-chan llmResponse {
	responseChan := make(chan llmResponse)

	go func() {
		defer close(responseChan)

		msgs := make([]api.Message, len(chats))
		for i, chat := range chats {
			msgs[i] = api.Message{
				Role:    chat.Role,
				Content: chat.Content,
			}
		}

		t := true
		req := api.ChatRequest{
			Model:    o.model,
			Messages: msgs,
			Stream:   &t,
			Options: map[string]interface{}{
				"temperature": o.temperature,
			},
		}

		if err := o.client.Chat(ctx, &req, func(res api.ChatResponse) error {
			responseChan <- llmResponse{
				content: res.Message.Content,
			}

			return nil
		}); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			responseChan <- llmResponse{
				err: fmt.Errorf("error sending request: %w", err),
			}
			return
		}
	}()

	return responseChan
}

// embeddingFunc returns an EmbeddingFunc that uses the OpenAI API to generate
// embeddings for the given text.
//
// This codes is taken directly from the chromem-go library, with a little modification,
// to make it work to the newer ollama embedding API.
func (o ollama) embeddingFunc() chromem.EmbeddingFunc {
	var checkedNormalized bool
	checkNormalized := sync.Once{}

	return func(ctx context.Context, text string) ([]float32, error) {
		req := api.EmbedRequest{
			Model: o.model,
			Input: text,
		}

		// Send the request.
		resp, err := o.client.Embed(ctx, &req)
		if err != nil {
			return nil, fmt.Errorf("couldn't send request: %w", err)
		}

		// Check if the response contains embeddings.
		if len(resp.Embeddings) == 0 {
			return nil, errors.New("no embeddings found in the response")
		}
		// In the newer ollama API, the request can take multiple inputs, and the response can contain multiple embeddings.
		// We only want the first embedding, so we take the first element of the array.
		if len(resp.Embeddings[0]) == 0 {
			return nil, errors.New("no embeddings found in the response")
		}

		v := resp.Embeddings[0]
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

func (o ollamaProvider) Title() string {
	if o.isConfigured() {
		return fmt.Sprintf("%s (configured)", providerOllama)
	}
	return fmt.Sprintf("%s (not configured)", providerOllama)
}

func (o ollamaProvider) Description() string {
	return "Configure Ollama connection"
}

func (o ollamaProvider) FilterValue() string {
	return providerOllama
}

func (ollamaProvider) name() string {
	return providerOllama
}

func (o ollamaProvider) availableModels() []string {
	u, err := url.Parse(o.Host)
	if err != nil {
		return []string{}
	}
	client := api.NewClient(u, &http.Client{})

	resp, err := client.List(context.Background())
	if err != nil {
		return []string{}
	}

	models := make([]string, len(resp.Models))
	for i, model := range resp.Models {
		models[i] = model.Name
	}

	return models
}

func (o ollamaProvider) isConfigured() bool {
	return o.Host != ""
}

func (o ollamaProvider) form(width, height int, keymap *huh.KeyMap) *huh.Form {
	host := o.Host
	if host == "" {
		host = os.Getenv("OLLAMA_HOST")
	}
	if host == "" {
		host = defaultOllamaHost
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("ollamaHost").
				Title("Host").
				Description("Enter the host for ollama.").
				Placeholder("Host").
				Value(&host),
			huh.NewConfirm().
				Key("ollamaConfirm").
				Title("Confirm").
				Description("Save this ollama settings?").
				Affirmative("Yes").
				Negative("Back"),
		),
	).
		WithWidth(width).
		WithHeight(height).
		WithTheme(huh.ThemeCatppuccin()).
		WithKeyMap(keymap).
		WithShowErrors(true).
		WithShowHelp(true)
}

func (o ollamaProvider) saveForm(db *bolt.DB, form *huh.Form) (llmProvider, bool, error) {
	if !form.GetBool("ollamaConfirm") {
		return o, false, nil
	}

	host := form.GetString("ollamaHost")
	if host == "" {
		return o, false, nil
	}

	o.Host = host

	if err := saveOllamaSettings(db, o); err != nil {
		return o, false, fmt.Errorf("error saving ollama settings: %w", err)
	}

	return o, true, nil
}

func (o ollamaProvider) new(setting llmSetting) llm {
	u, err := url.Parse(o.Host)
	if err != nil {
		panic(err)
	}

	return ollama{
		host:        o.Host,
		model:       setting.Model,
		temperature: setting.Temperature,
		client:      api.NewClient(u, &http.Client{}),
	}
}

func (o ollamaProvider) supportEmbedding() bool {
	return true
}

func (o ollamaProvider) newEmbedder(setting llmSetting) embedder {
	u, err := url.Parse(o.Host)
	if err != nil {
		panic(err)
	}
	return ollama{
		host:        o.Host,
		model:       setting.Model,
		temperature: setting.Temperature,
		client:      api.NewClient(u, &http.Client{}),
	}
}
