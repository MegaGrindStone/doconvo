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
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type anthropicProvider struct {
	APIKey string `json:"apiKey"`
}

type anthropic struct {
	apiKey      string
	model       string
	temperature float64

	client *http.Client
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

func (a anthropicProvider) newAnthropic(model string, temperature float64) anthropic {
	return anthropic{
		apiKey:      a.APIKey,
		model:       model,
		temperature: temperature,
		client:      &http.Client{},
	}
}

func (a anthropicProvider) isConfigured() bool {
	return a.APIKey != ""
}

func (m mainModel) newAnthropicForm() (mainModel, tea.Cmd) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	m.anthropicForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("anthropicAPIKey").
				Title("API Key").
				Description("Enter the API key for anthropic.").
				Placeholder("API Key").
				Value(&apiKey),
			huh.NewConfirm().
				Key("anthropicConfirm").
				Title("Confirm").
				Description("Save this anthropic settings?").
				Affirmative("Yes").
				Negative("Back"),
		),
	).
		WithWidth(m.formWidth).
		WithHeight(m.formHeight).
		WithKeyMap(m.keymap.formKeymap).
		WithShowErrors(true).
		WithShowHelp(true)

	return m, m.anthropicForm.PrevField()
}

func (m mainModel) updateAnthropicFormSize() mainModel {
	titleHeight := lipgloss.Height(titleStyle.Render(""))
	height := m.height - logoHeight() - titleHeight

	if m.err != nil {
		height -= errHeight(m.width, m.err)
	}

	m.formWidth = m.width
	m.formHeight = height

	return m
}

func (m mainModel) handleAnthropicFormEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.updateAnthropicFormSize()
	case tea.KeyMsg:
		if key.Matches(msg, m.keymap.escape) {
			return m.setViewState(viewStateOptions), nil
		}
	}

	form, cmd := m.anthropicForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.anthropicForm = f
	}

	if m.anthropicForm.State != huh.StateCompleted {
		return m, cmd
	}

	if !m.anthropicForm.GetBool("anthropicConfirm") {
		return m.setViewState(viewStateOptions), nil
	}

	apiKey := m.anthropicForm.GetString("anthropicAPIKey")
	if apiKey == "" {
		return m.setViewState(viewStateOptions), nil
	}

	m.llmProvider.anthropic.APIKey = apiKey

	if err := saveAnthropicSettings(m.db, m.llmProvider.anthropic); err != nil {
		m.err = fmt.Errorf("error saving anthropic settings: %w", err)
		return m.updateAnthropicFormSize(), nil
	}

	idx := slices.IndexFunc(optionItems, func(o optionItem) bool {
		return o.title == optionAnthropicTitle
	})
	oi := optionItems[idx]
	oi.title += " (configured)"
	m.optionsList.SetItem(idx, oi)

	return m.setViewState(viewStateOptions), nil
}

func (m mainModel) anthropicFormView() string {
	title := "Anthropic Settings"
	if m.llmProvider.anthropic.isConfigured() {
		title = "Edit Anthropic Settings"
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		logoView(),
		titleStyle.Render(title),
		m.anthropicForm.View(),
	)
}
