package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/philippgille/chromem-go"
	bolt "go.etcd.io/bbolt"
)

type mainModel struct {
	db       *bolt.DB
	vectordb *chromem.DB

	convoAI    ai
	genTitleAI ai
	embedderAI ai

	aiResponses            chan aiResponseMsg
	chatCancelFunc         context.CancelFunc
	documentScanProgress   chan documentScanLogMsg
	documentScanCancelFunc context.CancelFunc

	sessionList list.Model

	chatViewport   viewport.Model
	chatMDRenderer *glamour.TermRenderer
	chatSpinner    spinner.Model
	chatTextArea   textarea.Model

	optionsList list.Model

	documentsList        list.Model
	documentForm         *huh.Form
	documentScanViewport viewport.Model

	helpModel help.Model

	sessions              []session
	selectedSessionIndex  int
	chatIsThinking        bool
	documents             []document
	selectedDocumentIndex int
	documentScanLogs      []string
	documentScanStartTime time.Time

	keymap             keymap
	width              int
	height             int
	documentFormWidth  int
	documentFormHeight int

	viewState viewState
	err       error
}

type viewState int

const (
	viewStateSessions viewState = iota
	viewStateChat
	viewStateOptions
	viewStateDocuments
	viewStateDocumentForm
	viewStateDocumentScan
)

func main() {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(fmt.Errorf("error getting user option dir: %w", err))
	}

	cfgPath := filepath.Join(cfgDir, "/doconvo")
	if err := os.MkdirAll(cfgPath, 0755); err != nil {
		log.Fatal(fmt.Errorf("error creating option directory: %w", err))
	}

	dbPath := filepath.Join(cfgDir, "/doconvo/doconvo.db")
	vectordbPath := filepath.Join(cfgDir, "/doconvo/vectordb")

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		log.Fatal(fmt.Errorf("error opening database: %w", err))
	}
	defer db.Close()

	if err := initKVDB(db); err != nil {
		log.Fatal(fmt.Errorf("error initializing kvdb: %w", err))
	}

	vectordb, err := chromem.NewPersistentDB(vectordbPath, false)
	if err != nil {
		log.Fatal(fmt.Errorf("error opening vector database: %w", err))
	}

	m, err := newMainModel(db, vectordb)
	if err != nil {
		log.Fatal(fmt.Errorf("error initializing model: %w", err))
	}

	p := tea.NewProgram(m)

	go func() {
		for msg := range m.aiResponses {
			p.Send(aiResponseMsg(msg))
		}
	}()

	go func() {
		for msg := range m.documentScanProgress {
			p.Send(documentScanLogMsg(msg))
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}

func newMainModel(db *bolt.DB, vectordb *chromem.DB) (mainModel, error) {
	m := mainModel{
		db:       db,
		vectordb: vectordb,
	}
	ais := loadAI()
	m.convoAI = ais[convoName]
	m.genTitleAI = ais[titleGenName]
	m.embedderAI = ais[embedderName]

	m.keymap = newKeymap()

	var err error

	m, err = m.initSessions()
	if err != nil {
		return m, fmt.Errorf("error initializing sessions: %w", err)
	}
	m = m.initChat()
	m = m.initOptions()

	m, err = m.initDocuments()
	if err != nil {
		return m, fmt.Errorf("error initializing documents: %w", err)
	}
	m = m.initDocumentScan()

	m.helpModel = help.New()

	return m, nil
}

func (mainModel) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if key.Matches(msg, m.keymap.quit) {
			return m, tea.Quit
		}
	case aiResponseTitleMsg:
		// We put this handler here because this title generation message might
		// be received when viewState is not viewStateChat.
		return m.handleChatsResponseTitle(msg), nil
	}

	var cmd tea.Cmd

	switch m.viewState {
	case viewStateSessions:
		m, cmd = m.handleSessionsEvents(msg)
	case viewStateChat:
		m, cmd = m.handleChatEvents(msg)
	case viewStateOptions:
		m, cmd = m.handleOptionsEvents(msg)
	case viewStateDocuments:
		m, cmd = m.handleDocumentsEvents(msg)
	case viewStateDocumentForm:
		m, cmd = m.handleDocumentFormEvents(msg)
	case viewStateDocumentScan:
		m, cmd = m.handleDocumentScanEvents(msg)
	}

	return m, cmd
}

func (m mainModel) View() string {
	var vs []string

	switch m.viewState {
	case viewStateSessions:
		vs = append(vs, m.sessionsView())
	case viewStateChat:
		vs = append(vs, m.chatView())
	case viewStateOptions:
		vs = append(vs, m.optionsView())
	case viewStateDocuments:
		vs = append(vs, m.documentsView())
	case viewStateDocumentForm:
		vs = append(vs, m.documentFormView())
	case viewStateDocumentScan:
		vs = append(vs, m.documentScanView())
	default:
		m.err = fmt.Errorf("unknown view state %d", m.viewState)
	}

	if m.err != nil {
		vs = append(vs, errView(m.width, m.err))
	}

	return lipgloss.JoinVertical(lipgloss.Left, vs...)
}

func (m mainModel) setViewState(state viewState) mainModel {
	m.viewState = state
	m.keymap.viewState = state

	return m
}
