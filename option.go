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
	optionDocumentsTitle = "Documents"
)

var optionItems = []optionItem{
	{
		title:       optionDocumentsTitle,
		description: "Manages the documents you want to have convo with",
	},
}

func (m mainModel) initOptions() mainModel {
	items := make([]list.Item, len(optionItems))
	for i, item := range optionItems {
		items[i] = item
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
			return m.setViewState(viewStateSessions).updateSessionsSize(), nil
		case key.Matches(msg, m.keymap.pick):
			return m.selectOption(m.optionsList.Index()), nil
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

func (m mainModel) selectOption(index int) mainModel {
	option := optionItems[index]

	switch option.title {
	case optionDocumentsTitle:
		return m.setViewState(viewStateDocuments).updateDocumentsSize()
	}
	return m
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
