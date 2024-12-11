package main

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/philippgille/chromem-go"
)

type llmResponse struct {
	content string
	err     error
}

type llmResponseMsg struct {
	chatIndex  int
	content    string
	isThinking bool
	err        error
	done       bool
}

type llmResponseTitleMsg struct {
	title        string
	sessionIndex int
	err          error
}

type llmSetting struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
}

type llm interface {
	chat(context.Context, []chat) llmResponse
	chatStream(context.Context, []chat) <-chan llmResponse
}

type embedder interface {
	embeddingFunc() chromem.EmbeddingFunc
}

const (
	roleConvo    = "convo"
	roleTitleGen = "title-gen"
	roleEmbedder = "embedder"

	convoDefaultTemperature    = 0.8
	genTitleDefaultTemperature = 0.2
)

func extractSystemChat(chats []chat) (string, []chat) {
	if len(chats) == 0 {
		return "", chats
	}

	if chats[0].Role == roleSystem {
		return chats[0].Content, chats[1:]
	}

	return "", chats
}

func llmFromSetting(setting llmSetting, providers []llmProvider) (llm, error) {
	for _, p := range providers {
		if p.name() == setting.Provider {
			return p.new(setting), nil
		}
	}

	return nil, fmt.Errorf("unknown llm provider: %s", setting.Provider)
}

func embedderFromSetting(setting llmSetting, providers []llmProvider) (embedder, error) {
	for _, p := range providers {
		if !p.supportEmbedding() {
			continue
		}
		if p.name() == setting.Provider {
			return p.newEmbedder(setting), nil
		}
	}

	return nil, fmt.Errorf("unknown llm provider: %s", setting.Provider)
}

func (l llmSetting) isConfigured() bool {
	return l.Provider != "" && l.Model != ""
}

func (m mainModel) initLLMSettings() (mainModel, error) {
	var err error

	m.convoLLMSetting, err = loadLLMSettings(m.db, roleConvo)
	if err != nil {
		return mainModel{}, fmt.Errorf("failed to load convo llm settings: %w", err)
	}

	m.genTitleLLMSetting, err = loadLLMSettings(m.db, roleTitleGen)
	if err != nil {
		return mainModel{}, fmt.Errorf("failed to load title gen llm settings: %w", err)
	}

	m.embedderLLMSetting, err = loadLLMSettings(m.db, roleEmbedder)
	if err != nil {
		return mainModel{}, fmt.Errorf("failed to load embedder llm settings: %w", err)
	}

	return m, nil
}

func (m mainModel) newLLMForm(setting llmSetting, isEmbedding bool, defaultTemperature float64) *huh.Form {
	pIdx := slices.IndexFunc(m.providers, func(p llmProvider) bool {
		return p.name() == setting.Provider
	})
	var p llmProvider
	if pIdx > -1 {
		p = m.providers[pIdx]
	}
	mdl := setting.Model
	tmp := defaultTemperature
	if setting.Temperature != 0 {
		tmp = setting.Temperature
	}
	tmpStr := fmt.Sprintf("%.2f", tmp)

	var options []huh.Option[llmProvider]
	for _, p := range m.providers {
		if !p.isConfigured() {
			continue
		}
		if isEmbedding {
			if !p.supportEmbedding() {
				continue
			}
		}
		options = append(options, huh.NewOption(p.name(), p))
	}

	fields := []huh.Field{
		huh.NewSelect[llmProvider]().
			Key("llmProvider").
			Options(options...).
			Title("Provider").
			Description("Select the LLM provider").
			Value(&p).
			Height(5),
		huh.NewSelect[string]().
			Key("llmModel").
			OptionsFunc(func() []huh.Option[string] {
				models := p.availableModels()
				return huh.NewOptions(models...)
			}, &p).
			Title("Model").
			Description("Select the LLM model").
			Value(&mdl).
			Height(10),
	}

	if !isEmbedding {
		fields = append(fields, huh.NewInput().
			Key("llmTemperature").
			Title("Temperature").
			Description("Enter the temperature").
			Placeholder("Temperature").
			Value(&tmpStr))
	}

	fields = append(fields,
		huh.NewConfirm().
			Key("llmConfirm").
			Title("Confirm").
			Description("Save this LLM settings?").
			Affirmative("Yes").
			Negative("Back"),
	)

	return huh.NewForm(
		huh.NewGroup(
			fields...,
		),
	).
		WithWidth(m.formWidth).
		WithHeight(m.formHeight).
		WithTheme(huh.ThemeCatppuccin()).
		WithKeyMap(m.keymap.formKeymap).
		WithShowErrors(true).
		WithShowHelp(true)
}

func (m mainModel) llmIsConfigured() bool {
	if !m.convoLLMSetting.isConfigured() {
		return false
	}
	if !m.genTitleLLMSetting.isConfigured() {
		return false
	}
	if !m.embedderLLMSetting.isConfigured() {
		return false
	}

	return true
}

func (m mainModel) newConvoLLMForm() (mainModel, tea.Cmd) {
	m.convoLLMForm = m.newLLMForm(m.convoLLMSetting, false, convoDefaultTemperature)

	return m, m.convoLLMForm.PrevField()
}

func (m mainModel) handleConvoLLMFormEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.updateFormSize()
	case tea.KeyMsg:
		if key.Matches(msg, m.keymap.escape) {
			return m.setViewState(viewStateOptions), nil
		}
	}

	form, cmd := m.convoLLMForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.convoLLMForm = f
	}

	if m.convoLLMForm.State != huh.StateCompleted {
		return m, cmd
	}

	if !m.convoLLMForm.GetBool("llmConfirm") {
		return m, nil
	}

	p, _ := m.convoLLMForm.Get("llmProvider").(llmProvider)
	m.convoLLMSetting.Provider = p.name()
	m.convoLLMSetting.Model = m.convoLLMForm.GetString("llmModel")
	tmp, err := strconv.ParseFloat(m.convoLLMForm.GetString("llmTemperature"), 64)
	if err != nil {
		tmp = 0
	}
	m.convoLLMSetting.Temperature = tmp

	if err := saveLLMSettings(m.db, roleConvo, m.convoLLMSetting); err != nil {
		m.err = fmt.Errorf("error saving convo llm settings: %w", err)
		slog.Error(m.err.Error())
		return m.updateFormSize(), nil
	}

	m, err = m.refreshRAG()
	if err != nil {
		m.err = fmt.Errorf("error refreshing rag: %w", err)
		slog.Error(m.err.Error())
		return m.updateFormSize(), nil
	}

	return m.initOptions().updateOptionsSize().setViewState(viewStateOptions), nil
}

func (m mainModel) convoLLMFormView() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		logoView(),
		titleStyle.Render("Convo LLM"),
		m.convoLLMForm.View(),
	)
}

func (m mainModel) newGenTitleLLMForm() (mainModel, tea.Cmd) {
	m.genTitleLLMForm = m.newLLMForm(m.genTitleLLMSetting, false, genTitleDefaultTemperature)

	return m, m.genTitleLLMForm.PrevField()
}

func (m mainModel) handleGenTitleLLMFormEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.updateFormSize()
	case tea.KeyMsg:
		if key.Matches(msg, m.keymap.escape) {
			return m.setViewState(viewStateOptions), nil
		}
	}

	form, cmd := m.genTitleLLMForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.genTitleLLMForm = f
	}

	if m.genTitleLLMForm.State != huh.StateCompleted {
		return m, cmd
	}

	if !m.genTitleLLMForm.GetBool("llmConfirm") {
		return m, nil
	}

	p, _ := m.genTitleLLMForm.Get("llmProvider").(llmProvider)
	m.genTitleLLMSetting.Provider = p.name()
	m.genTitleLLMSetting.Model = m.genTitleLLMForm.GetString("llmModel")
	tmp, err := strconv.ParseFloat(m.genTitleLLMForm.GetString("llmTemperature"), 64)
	if err != nil {
		tmp = 0
	}
	m.genTitleLLMSetting.Temperature = tmp

	if err := saveLLMSettings(m.db, roleTitleGen, m.genTitleLLMSetting); err != nil {
		m.err = fmt.Errorf("error saving gen title llm settings: %w", err)
		slog.Error(m.err.Error())
		return m.updateFormSize(), nil
	}

	m, err = m.refreshRAG()
	if err != nil {
		m.err = fmt.Errorf("error refreshing rag: %w", err)
		slog.Error(m.err.Error())
		return m.updateFormSize(), nil
	}

	return m.initOptions().updateOptionsSize().setViewState(viewStateOptions), nil
}

func (m mainModel) genTitleLLMFormView() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		logoView(),
		titleStyle.Render("Generate Title LLM"),
		m.genTitleLLMForm.View(),
	)
}

func (m mainModel) newEmbedderLLMForm() (mainModel, tea.Cmd) {
	m.embedderLLMForm = m.newLLMForm(m.embedderLLMSetting, true, 0)

	return m, m.embedderLLMForm.PrevField()
}

func (m mainModel) handleEmbedderLLMFormEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.updateFormSize()
	case tea.KeyMsg:
		if key.Matches(msg, m.keymap.escape) {
			return m.setViewState(viewStateOptions), nil
		}
	}

	form, cmd := m.embedderLLMForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.embedderLLMForm = f
	}

	if m.embedderLLMForm.State != huh.StateCompleted {
		return m, cmd
	}

	if !m.embedderLLMForm.GetBool("llmConfirm") {
		return m, nil
	}

	p, _ := m.embedderLLMForm.Get("llmProvider").(llmProvider)
	m.embedderLLMSetting.Provider = p.name()
	m.embedderLLMSetting.Model = m.embedderLLMForm.GetString("llmModel")

	if err := saveLLMSettings(m.db, roleEmbedder, m.embedderLLMSetting); err != nil {
		m.err = fmt.Errorf("error saving embedder llm settings: %w", err)
		slog.Error(m.err.Error())
		return m.updateFormSize(), nil
	}

	var err error
	m, err = m.refreshRAG()
	if err != nil {
		m.err = fmt.Errorf("error refreshing rag: %w", err)
		slog.Error(m.err.Error())
		return m.updateFormSize(), nil
	}

	return m.initOptions().updateOptionsSize().setViewState(viewStateOptions), nil
}

func (m mainModel) embedderLLMFormView() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		logoView(),
		titleStyle.Render("Embedder LLM"),
		m.embedderLLMForm.View(),
	)
}
