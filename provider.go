package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	bolt "go.etcd.io/bbolt"
)

type llmProvider interface {
	name() string
	availableModels() []string
	isConfigured() bool

	form(int, int, *huh.KeyMap) *huh.Form
	saveForm(*bolt.DB, *huh.Form) (llmProvider, bool, error)

	Title() string
	Description() string
	list.Item

	new(llmSetting) llm

	supportEmbedding() bool
	newEmbedder(llmSetting) embedder
}

const (
	providerOllama    = "Ollama"
	providerAnthropic = "Anthropic"
	providerOpenAI    = "OpenAI"
)

func loadLLMProviders(db *bolt.DB) ([]llmProvider, error) {
	o, err := loadOllamaSettings(db)
	if err != nil {
		return nil, fmt.Errorf("failed to load ollama settings: %w", err)
	}
	a, err := loadAnthropicSettings(db)
	if err != nil {
		return nil, fmt.Errorf("failed to load anthropic settings: %w", err)
	}
	oa, err := loadOpenAISettings(db)
	if err != nil {
		return nil, fmt.Errorf("failed to load openai settings: %w", err)
	}

	return []llmProvider{o, a, oa}, nil
}

func (m mainModel) providersIsConfigured() bool {
	for _, p := range m.providers {
		if p.isConfigured() {
			return true
		}
	}
	return false
}

func (m mainModel) initProviders() (mainModel, error) {
	var err error
	m.providers, err = loadLLMProviders(m.db)
	if err != nil {
		return mainModel{}, fmt.Errorf("failed to load llm providers: %w", err)
	}

	items := make([]list.Item, 0, len(m.providers))
	for _, item := range m.providers {
		items = append(items, item)
	}

	m.providersList = defaultList("Providers", m.keymap, func() []key.Binding {
		return []key.Binding{
			m.keymap.escape,
		}
	}, func() []key.Binding {
		return []key.Binding{
			m.keymap.pick,
			m.keymap.escape,
		}
	})
	m.providersList.SetItems(items)
	m.providersList.SetFilteringEnabled(false)
	m.providersList.SetShowStatusBar(false)

	return m, nil
}

func (m mainModel) updateProvidersSize() mainModel {
	height := m.height - logoHeight()

	if m.err != nil {
		height -= errHeight(m.width, m.err)
	}

	m.providersList.SetSize(m.width, height)
	return m
}

func (m mainModel) handleProvidersEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.updateProvidersSize()
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.escape):
			return m.setViewState(viewStateOptions).updateOptionsSize(), nil
		case key.Matches(msg, m.keymap.pick):
			return m.selectProvider(m.providersList.Index())
		}
	}
	var cmd tea.Cmd
	m.providersList, cmd = m.providersList.Update(msg)
	return m, cmd
}

func (m mainModel) providersView() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		logoView(),
		m.providersList.View(),
	)
}

func (m mainModel) selectProvider(index int) (mainModel, tea.Cmd) {
	m.selectedProviderIndex = index

	return m.setViewState(viewStateProviderForm).
		updateFormSize().
		newProviderForm()
}

func (m mainModel) newProviderForm() (mainModel, tea.Cmd) {
	selectedProvider := m.providers[m.selectedProviderIndex]

	m.providerForm = selectedProvider.form(m.formWidth, m.formHeight, m.keymap.formKeymap)

	return m, m.providerForm.PrevField()
}

func (m mainModel) handleProviderFormEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.updateFormSize()
	case tea.KeyMsg:
		if key.Matches(msg, m.keymap.escape) {
			return m.setViewState(viewStateProviders), nil
		}
	}

	form, cmd := m.providerForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.providerForm = f
	}

	if m.providerForm.State != huh.StateCompleted {
		return m, cmd
	}

	provider, confirmed, err := m.providers[m.selectedProviderIndex].saveForm(m.db, m.providerForm)
	if err != nil {
		m.err = fmt.Errorf("error saving provider settings: %w", err)
		return m.updateFormSize(), nil
	}

	if !confirmed {
		return m.setViewState(viewStateProviders), nil
	}

	m.providers[m.selectedProviderIndex] = provider
	m.providersList.SetItem(m.selectedProviderIndex, provider)

	// We need to refresh the optionsList
	return m.initOptions().setViewState(viewStateProviders), nil
}

func (m mainModel) providerFormView() string {
	selectedProvider := m.providers[m.selectedProviderIndex]
	title := selectedProvider.Title()
	if selectedProvider.isConfigured() {
		title = "Edit " + title
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		logoView(),
		titleStyle.Render(title),
		m.providerForm.View(),
	)
}
