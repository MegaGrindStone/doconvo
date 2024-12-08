package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

type chat struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Failed    bool      `json:"failed"`
}

const (
	roleUser      = "user"
	roleAssistant = "assistant"
	roleSystem    = "system"
)

func (m mainModel) initChat() mainModel {
	m.chatViewport = viewport.New(0, 0)
	m.chatViewport.KeyMap = m.keymap.viewportKeymap

	m.chatSpinner = spinner.New(spinner.WithSpinner(spinner.MiniDot))

	m.chatTextArea = textarea.New()
	m.chatTextArea.SetPromptFunc(2, func(lineIdx int) string {
		if lineIdx == 0 {
			return "> "
		}
		return ""
	})
	m.chatTextArea.ShowLineNumbers = false
	m.chatTextArea.SetHeight(3)
	m.chatTextArea.Placeholder = "Type your message here..."
	m.chatTextArea.CharLimit = 0
	m.chatTextArea.KeyMap = m.keymap.textAreaKeymap

	m.chatMDRenderer, _ = glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithPreservedNewLines(),
		glamour.WithWordWrap(0),
	)

	m.llmResponses = make(chan llmResponseMsg)

	return m
}

func (m mainModel) updateChatSize() mainModel {
	selectedSession := m.sessions[m.selectedSessionIndex]

	titleHeight := lipgloss.Height(titleStyle.Render(selectedSession.Name))
	textareaHeight := lipgloss.Height(chatTextareaStyle.Render(m.chatTextArea.View()))
	helpHeight := lipgloss.Height(m.helpModel.View(m.keymap))

	newHeight := m.height - titleHeight - textareaHeight - helpHeight
	if m.err != nil {
		newHeight -= errHeight(m.width, m.err)
	}
	m.chatViewport.Height = newHeight

	m.chatTextArea.SetWidth(m.width - chatTextareaStyle.GetHorizontalFrameSize())

	var sb strings.Builder
	for _, c := range selectedSession.Chats {
		rc, _ := m.chatMDRenderer.Render(wordwrap.String(c.Content, m.width-10))

		sb.WriteString(chatEntityStyle.Render(fmt.Sprintf("%s: ", c.displayName())))
		sb.WriteString(chatContentStyle.Render(rc))
		sb.WriteString("\n")
	}
	if m.chatIsThinking {
		sb.WriteString(spinnerStyle.Render(m.chatSpinner.View()))
	}

	m.chatViewport.SetContent(sb.String())
	m.chatViewport.GotoBottom()

	return m
}

func (m mainModel) handleChatEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if m.viewState == viewStateChat {
			m = m.updateChatSize()
		}
	case tea.KeyMsg:
		if m.viewState != viewStateChat {
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keymap.escape):
			if m.chatCancelFunc != nil {
				m.chatCancelFunc()
				m.chatCancelFunc = nil
				return m, nil
			}

			m.err = nil
			return m.setViewState(viewStateSessions).updateSessionsSize(), nil
		case key.Matches(msg, m.keymap.submit):
			return m.sendChat()
		case key.Matches(msg, m.keymap.openHelp):
			m.keymap.openHelp.SetEnabled(false)
			m.keymap.closeHelp.SetEnabled(true)
			m.helpModel.ShowAll = true
			return m.updateChatSize(), nil
		case key.Matches(msg, m.keymap.closeHelp):
			m.keymap.closeHelp.SetEnabled(false)
			m.keymap.openHelp.SetEnabled(true)
			m.helpModel.ShowAll = false
			return m.updateChatSize(), nil
		}
	case spinner.TickMsg:
		if !m.chatIsThinking {
			// Stop the spinner if the LLM is not thinking anymore
			return m, nil
		}
		// Updating the spinner here would cause the spinner to tick again
		var cmd tea.Cmd
		m.chatSpinner, cmd = m.chatSpinner.Update(msg)
		return m.updateChatSize(), cmd
	case llmResponseMsg:
		return m.handleChatsResponse(msg)
	}

	m.chatTextArea, cmd = m.chatTextArea.Update(msg)
	cmds = append(cmds, cmd)

	m.chatSpinner, cmd = m.chatSpinner.Update(msg)
	cmds = append(cmds, cmd)

	m.chatViewport, cmd = m.chatViewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m mainModel) handleChatsResponse(msg llmResponseMsg) (mainModel, tea.Cmd) {
	selectedSession := m.sessions[m.selectedSessionIndex]

	if msg.chatIndex == len(selectedSession.Chats) {
		selectedSession.Chats = append(selectedSession.Chats, chat{
			Role:      roleAssistant,
			Timestamp: time.Now(),
		})
	}

	if msg.err != nil {
		if !errors.Is(msg.err, context.Canceled) {
			selectedSession.Chats[len(selectedSession.Chats)-1].
				Content = "Sorry, I'm having trouble connecting to the LLM. Please try again later."
			selectedSession.Chats[len(selectedSession.Chats)-1].Failed = true
		}
		m.sessions[m.selectedSessionIndex] = selectedSession

		m.chatIsThinking = false
		m.chatTextArea.Focus()
		m.err = msg.err
		if err := saveSession(m.db, &selectedSession); err != nil {
			m.err = fmt.Errorf("error saving session: %w", err)
		}
		return m.updateChatSize(), nil
	}

	m.chatIsThinking = msg.isThinking
	selectedSession.Chats[len(selectedSession.Chats)-1].Content += msg.content

	var cmds []tea.Cmd
	var cmd tea.Cmd

	if msg.done {
		m.chatIsThinking = false
		m.chatCancelFunc = nil
		if selectedSession.Name == "" {
			sessionIndex := m.selectedSessionIndex
			cmd = func() tea.Msg {
				name, err := m.rag.genTitle()
				if err != nil {
					return llmResponseTitleMsg{
						err: fmt.Errorf("error generating session title: %w", err),
					}
				}
				return llmResponseTitleMsg{
					title:        name,
					sessionIndex: sessionIndex,
				}
			}
			cmds = append(cmds, cmd)
		}
		m.chatTextArea.Focus()
	}
	m.sessions[m.selectedSessionIndex] = selectedSession
	if err := saveSession(m.db, &selectedSession); err != nil {
		m.err = fmt.Errorf("error saving session: %w", err)
	}

	return m.updateChatSize(), tea.Batch(cmds...)
}

func (m mainModel) handleChatsResponseTitle(msg llmResponseTitleMsg) mainModel {
	if msg.err != nil {
		m.err = msg.err
		return m.updateChatSize()
	}

	// We use the session index from the message to ensure we're updating the correct session,
	// because the session selection might have changed due to the user's actions.
	selectedSession := m.sessions[msg.sessionIndex]
	selectedSession.Name = msg.title

	m.sessions[msg.sessionIndex] = selectedSession
	m.sessionList.SetItem(msg.sessionIndex, selectedSession)

	if err := saveSession(m.db, &selectedSession); err != nil {
		m.err = fmt.Errorf("error saving session: %w", err)
		return m.updateChatSize()
	}

	return m
}

func (m mainModel) chatView() string {
	selectedSession := m.sessions[m.selectedSessionIndex]

	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(selectedSession.Title()),
		m.chatViewport.View(),
		chatTextareaStyle.Render(m.chatTextArea.View()),
		m.helpModel.View(m.keymap),
	)
}

func (m mainModel) sendChat() (mainModel, tea.Cmd) {
	if m.chatTextArea.Value() == "" {
		return m, nil
	}

	msg := m.chatTextArea.Value()
	selectedSession := m.sessions[m.selectedSessionIndex]

	m.err = nil
	selectedSession.Chats = append(selectedSession.Chats, chat{
		Role:      roleUser,
		Content:   msg,
		Timestamp: time.Now(),
	})
	m.chatIsThinking = true
	m.chatTextArea.Reset()
	m.chatTextArea.Blur()

	ctx, cancel := context.WithCancel(context.Background())
	m.chatCancelFunc = cancel

	go m.rag.chat(ctx, msg, len(selectedSession.Chats), m.documents, m.llmResponses)

	m.sessions[m.selectedSessionIndex] = selectedSession

	return m.updateChatSize(), func() tea.Msg {
		return m.chatSpinner.Tick()
	}
}

func (c chat) displayName() string {
	if c.Role == roleUser {
		return "You"
	}
	if c.Role == roleAssistant {
		return "Assistant"
	}

	return "System"
}
