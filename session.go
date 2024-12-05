package main

import (
	"fmt"
	"slices"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type session struct {
	ID      int       `json:"id"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`

	Chats []chat `json:"chats"`
}

func (m mainModel) initSessions() (mainModel, error) {
	var err error
	m.sessions, err = loadSessions(m.db)
	if err != nil {
		return mainModel{}, fmt.Errorf("failed to load sessions: %w", err)
	}

	items := make([]list.Item, len(m.sessions))
	for i, item := range m.sessions {
		items[i] = item
	}

	m.sessionList = defaultList("Sessions List", m.keymap, func() []key.Binding {
		return []key.Binding{
			m.keymap.new,
		}
	}, func() []key.Binding {
		return []key.Binding{
			m.keymap.new,
			m.keymap.delete,
			m.keymap.pick,
		}
	})
	m.sessionList.SetItems(items)

	return m, nil
}

func (m mainModel) updateSessionsSize() mainModel {
	height := m.height - logoHeight()

	if m.err != nil {
		height -= errHeight(m.width, m.err)
	}

	m.sessionList.SetSize(m.width, height)
	return m
}

func (m mainModel) handleSessionsEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.updateSessionsSize()
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.new):
			return m.newSession()
		case key.Matches(msg, m.keymap.delete):
			return m.deleteSession(m.sessionList.Index()), nil
		case key.Matches(msg, m.keymap.pick):
			return m.selectSession(m.sessionList.Index()), nil
		}
	}

	var cmd tea.Cmd
	m.sessionList, cmd = m.sessionList.Update(msg)
	return m, cmd
}

func (m mainModel) sessionsView() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		logoView(),
		m.sessionList.View(),
	)
}

func (m mainModel) newSession() (mainModel, tea.Cmd) {
	newSession := session{
		Created: time.Now(),
		Chats:   []chat{},
	}
	if err := saveSession(m.db, &newSession); err != nil {
		m.err = fmt.Errorf("error creating new session: %w", err)
		return m.updateSessionsSize(), nil
	}
	m.sessions = append(m.sessions, newSession)
	newIndex := len(m.sessions) - 1

	// If we directly return this InsertItem command, the session list will not be
	// updated. This is because the updated list won't be picked up by the copy
	// of the model returned by the m.selectSession below, that's why we need to
	// make sure this command is executed and updated in the main model.
	cmd := m.sessionList.InsertItem(newIndex, newSession)

	return m.selectSession(newIndex), cmd
}

func (m mainModel) selectSession(index int) mainModel {
	m.selectedSessionIndex = index
	m.chatTextArea.Reset()
	m.chatTextArea.Focus()

	return m.setViewState(viewStateChat).updateChatSize()
}

func (m mainModel) deleteSession(index int) mainModel {
	session := m.sessions[index]

	if err := deleteSession(m.db, session.ID); err != nil {
		m.err = fmt.Errorf("error deleting session: %w", err)
		return m.updateSessionsSize()
	}

	m.sessions = slices.Delete(m.sessions, index, index+1)
	m.sessionList.RemoveItem(index)

	return m
}

func (s session) Title() string {
	if s.Name == "" {
		return "Untitled"
	}
	return s.Name
}

func (s session) Description() string {
	return s.Created.Format(time.RFC1123)
}

func (s session) FilterValue() string {
	return s.Name
}
