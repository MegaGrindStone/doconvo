package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/philippgille/chromem-go"
	goopenai "github.com/sashabaranov/go-openai"
	bolt "go.etcd.io/bbolt"
)

type openaiProvider struct {
	APIKey string `json:"apiKey"`
}

type openai struct {
	apiKey      string
	model       string
	temperature float64

	client *goopenai.Client
}

func (o openai) chat(ctx context.Context, chats []chat) llmResponse {
	systemChat, cs := extractSystemChat(chats)

	msgs := make([]goopenai.ChatCompletionMessage, 0, len(cs)+1)
	if systemChat != "" {
		msgs = append(msgs, goopenai.ChatCompletionMessage{
			Role:    goopenai.ChatMessageRoleSystem,
			Content: systemChat,
		})
	}

	for _, chat := range cs {
		msgs = append(msgs, goopenai.ChatCompletionMessage{
			Role:    chat.Role,
			Content: chat.Content,
		})
	}

	resp, err := o.client.CreateChatCompletion(
		ctx,
		goopenai.ChatCompletionRequest{
			Model:       o.model,
			Messages:    msgs,
			Temperature: float32(o.temperature),
		},
	)
	if err != nil {
		return llmResponse{
			err: fmt.Errorf("error creating chat completion: %w", err),
		}
	}

	if len(resp.Choices) == 0 {
		return llmResponse{
			err: fmt.Errorf("no choices in response"),
		}
	}

	return llmResponse{
		content: resp.Choices[0].Message.Content,
	}
}

func (o openai) chatStream(ctx context.Context, chats []chat) <-chan llmResponse {
	responseChan := make(chan llmResponse)

	go func() {
		defer close(responseChan)

		systemChat, cs := extractSystemChat(chats)

		msgs := make([]goopenai.ChatCompletionMessage, 0, len(cs)+1)
		if systemChat != "" {
			msgs = append(msgs, goopenai.ChatCompletionMessage{
				Role:    goopenai.ChatMessageRoleSystem,
				Content: systemChat,
			})
		}

		for _, chat := range cs {
			msgs = append(msgs, goopenai.ChatCompletionMessage{
				Role:    chat.Role,
				Content: chat.Content,
			})
		}

		stream, err := o.client.CreateChatCompletionStream(
			ctx,
			goopenai.ChatCompletionRequest{
				Model:       o.model,
				Messages:    msgs,
				Temperature: float32(o.temperature),
				Stream:      true,
			},
		)
		if err != nil {
			responseChan <- llmResponse{
				err: fmt.Errorf("error creating chat completion stream: %w", err),
			}
			return
		}
		defer stream.Close()

		for {
			response, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					return
				}
				if err == context.Canceled {
					return
				}
				responseChan <- llmResponse{
					err: fmt.Errorf("error receiving from stream: %w", err),
				}
				return
			}

			if len(response.Choices) > 0 && response.Choices[0].Delta.Content != "" {
				responseChan <- llmResponse{
					content: response.Choices[0].Delta.Content,
				}
			}
		}
	}()

	return responseChan
}

func (o openai) embeddingFunc() chromem.EmbeddingFunc {
	return chromem.NewEmbeddingFuncOpenAI(o.apiKey, chromem.EmbeddingModelOpenAI(o.model))
}

func (o openaiProvider) Title() string {
	if o.isConfigured() {
		return fmt.Sprintf("%s (configured)", providerOpenAI)
	}
	return fmt.Sprintf("%s (not configured)", providerOpenAI)
}

func (o openaiProvider) Description() string {
	return "Configure OpenAI connection"
}

func (o openaiProvider) FilterValue() string {
	return providerOpenAI
}

func (o openaiProvider) name() string {
	return providerOpenAI
}

func (o openaiProvider) availableModels() []string {
	client := goopenai.NewClient(o.APIKey)

	mList, err := client.ListModels(context.Background())
	if err != nil {
		return []string{}
	}

	res := make([]string, len(mList.Models))
	for i, m := range mList.Models {
		res[i] = m.ID
	}

	return res
}

func (o openaiProvider) isConfigured() bool {
	return o.APIKey != ""
}

func (o openaiProvider) form(width, height int, keymap *huh.KeyMap) *huh.Form {
	apiKey := o.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("openaiAPIKey").
				Title("API Key").
				Description("Enter the API key for OpenAI.").
				Placeholder("API Key").
				Value(&apiKey),
			huh.NewConfirm().
				Key("openaiConfirm").
				Title("Confirm").
				Description("Save this OpenAI settings?").
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

func (o openaiProvider) saveForm(db *bolt.DB, form *huh.Form) (llmProvider, bool, error) {
	if !form.GetBool("openaiConfirm") {
		return o, false, nil
	}

	apiKey := form.GetString("openaiAPIKey")

	if apiKey == "" {
		return o, false, nil
	}

	o.APIKey = apiKey

	if err := saveOpenAISettings(db, o); err != nil {
		return o, false, fmt.Errorf("error saving openai settings: %w", err)
	}

	return o, true, nil
}

func (o openaiProvider) new(setting llmSetting) llm {
	client := goopenai.NewClient(o.APIKey)
	return openai{
		apiKey:      o.APIKey,
		model:       setting.Model,
		temperature: setting.Temperature,
		client:      client,
	}
}

func (o openaiProvider) supportEmbedding() bool {
	return true
}

func (o openaiProvider) newEmbedder(setting llmSetting) embedder {
	client := goopenai.NewClient(o.APIKey)
	return &openai{
		apiKey: o.APIKey,
		model:  setting.Model,
		client: client,
	}
}
