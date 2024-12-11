package main

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type optionItem struct {
	title       string
	description string
}

const (
	optionDocumentsTitle   = "Documents"
	optionProvidersTitle   = "Providers"
	optionConvoLLMTitle    = "Convo LLM"
	optionGenTitleLLMTitle = "Generate Title LLM"
	optionEmbedderTitle    = "Embedder LLM"
)

var llmOptionItems = []optionItem{
	{
		title:       optionConvoLLMTitle,
		description: "The LLM to use for the conversation",
	},
	{
		title:       optionGenTitleLLMTitle,
		description: "The LLM to use for generating the title",
	},
	{
		title:       optionEmbedderTitle,
		description: "The LLM to use for embedding the document",
	},
}

func (m mainModel) initOptions() mainModel {
	m.options = make([]optionItem, 0)

	if m.embedderLLMSetting.isConfigured() {
		m.options = append(m.options, optionItem{
			title:       optionDocumentsTitle,
			description: "Manages the documents you want to have convo with",
		})
	}
	m.options = append(m.options, optionItem{
		title:       optionProvidersTitle,
		description: "Manages the LLM providers you want to use",
	})

	if m.providersIsConfigured() {
		m.options = append(m.options, llmOptionItems...)
	}

	items := make([]list.Item, len(m.options))
	for i, item := range m.options {
		it := item

		switch item.title {
		case optionProvidersTitle:
			if m.providersIsConfigured() {
				it.title += " (configured)"
			} else {
				it.title += " (not configured)"
			}
		case optionConvoLLMTitle:
			if m.convoLLMSetting.isConfigured() {
				it.title += " (configured)"
			} else {
				it.title += " (not configured)"
			}
		case optionGenTitleLLMTitle:
			if m.genTitleLLMSetting.isConfigured() {
				it.title += " (configured)"
			} else {
				it.title += " (not configured)"
			}
		case optionEmbedderTitle:
			if m.embedderLLMSetting.isConfigured() {
				it.title += " (configured)"
			} else {
				it.title += " (not configured)"
			}
		}

		items[i] = it
	}

	m.optionsList = defaultList("Options", m.keymap, func() []key.Binding {
		return []key.Binding{
			m.keymap.escape,
		}
	}, func() []key.Binding {
		return []key.Binding{
			m.keymap.pick,
			m.keymap.escape,
		}
	})
	m.optionsList.SetItems(items)
	m.optionsList.SetFilteringEnabled(false)
	m.optionsList.SetShowStatusBar(false)

	return m
}

func (m mainModel) updateOptionsSize() mainModel {
	height := m.height - logoHeight()

	if m.err != nil {
		height -= errHeight(m.width, m.err)
	}

	m.optionsList.SetSize(m.width, height)
	return m
}

func (m mainModel) handleOptionsEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.updateOptionsSize()
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.escape):
			if !m.providersIsConfigured() || !m.llmIsConfigured() {
				return m, nil
			}
			return m.setViewState(viewStateSessions).updateSessionsSize(), nil
		case key.Matches(msg, m.keymap.pick):
			return m.selectOption(m.optionsList.Index())
		}
	}
	var cmd tea.Cmd
	m.optionsList, cmd = m.optionsList.Update(msg)
	return m, cmd
}

func (m mainModel) optionsView() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		logoView(),
		m.optionsList.View(),
	)
}

func (m mainModel) selectOption(index int) (mainModel, tea.Cmd) {
	option := m.options[index]

	switch option.title {
	case optionDocumentsTitle:
		return m.setViewState(viewStateDocuments).updateDocumentsSize(), nil
	case optionProvidersTitle:
		return m.setViewState(viewStateProviders).updateProvidersSize(), nil
	case optionConvoLLMTitle:
		return m.setViewState(viewStateConvoLLMForm).updateFormSize().newConvoLLMForm()
	case optionGenTitleLLMTitle:
		return m.setViewState(viewStateGenTitleLLMForm).updateFormSize().newGenTitleLLMForm()
	case optionEmbedderTitle:
		return m.setViewState(viewStateEmbedderLLMForm).updateFormSize().newEmbedderLLMForm()
	}
	return m, nil
}

func (c optionItem) Title() string {
	return c.title
}

func (c optionItem) Description() string {
	return c.description
}

func (c optionItem) FilterValue() string {
	return c.title
}
