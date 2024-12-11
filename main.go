package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
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
	rag      *rag

	llmResponses           chan llmResponseMsg
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

	providersList list.Model
	providerForm  *huh.Form

	convoLLMForm    *huh.Form
	genTitleLLMForm *huh.Form
	embedderLLMForm *huh.Form

	helpModel help.Model

	sessions              []session
	selectedSessionIndex  int
	chatIsThinking        bool
	options               []optionItem
	documents             []document
	selectedDocumentIndex int
	documentScanLogs      []string
	documentScanStartTime time.Time
	providers             []llmProvider
	selectedProviderIndex int
	convoLLMSetting       llmSetting
	genTitleLLMSetting    llmSetting
	embedderLLMSetting    llmSetting

	keymap     keymap
	width      int
	height     int
	formWidth  int
	formHeight int

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
	viewStateProviders
	viewStateProviderForm
	viewStateConvoLLMForm
	viewStateGenTitleLLMForm
	viewStateEmbedderLLMForm
)

func initLogger(cfgPath string) error {
	logPath := filepath.Join(cfgPath, "doconvo.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error creating log file: %w", err)
	}

	opts := &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	}

	handler := slog.NewJSONHandler(logFile, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return nil
}

func main() {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(fmt.Errorf("error getting user option dir: %w", err))
	}

	cfgPath := filepath.Join(cfgDir, "/doconvo")
	if err := os.MkdirAll(cfgPath, 0755); err != nil {
		log.Fatal(fmt.Errorf("error creating option directory: %w", err))
	}

	if err := initLogger(cfgPath); err != nil {
		log.Fatal(fmt.Errorf("error initializing logger: %w", err))
	}
	slog.Info("starting doconvo application")

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
		for msg := range m.llmResponses {
			p.Send(llmResponseMsg(msg))
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

	var err error

	m, err = m.initProviders()
	if err != nil {
		return mainModel{}, fmt.Errorf("failed to load llm providers: %w", err)
	}

	m, err = m.initLLMSettings()
	if err != nil {
		return mainModel{}, fmt.Errorf("failed to load llm settings: %w", err)
	}

	m.viewState = viewStateSessions
	if !m.providersIsConfigured() || !m.llmIsConfigured() {
		m.viewState = viewStateOptions
	}

	m, err = m.refreshRAG()
	if err != nil {
		return m, fmt.Errorf("failed to refresh rag: %w", err)
	}

	m.keymap = newKeymap()

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
	case llmResponseTitleMsg:
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
	case viewStateProviders:
		m, cmd = m.handleProvidersEvents(msg)
	case viewStateProviderForm:
		m, cmd = m.handleProviderFormEvents(msg)
	case viewStateConvoLLMForm:
		m, cmd = m.handleConvoLLMFormEvents(msg)
	case viewStateGenTitleLLMForm:
		m, cmd = m.handleGenTitleLLMFormEvents(msg)
	case viewStateEmbedderLLMForm:
		m, cmd = m.handleEmbedderLLMFormEvents(msg)
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
	case viewStateProviders:
		vs = append(vs, m.providersView())
	case viewStateProviderForm:
		vs = append(vs, m.providerFormView())
	case viewStateConvoLLMForm:
		vs = append(vs, m.convoLLMFormView())
	case viewStateGenTitleLLMForm:
		vs = append(vs, m.genTitleLLMFormView())
	case viewStateEmbedderLLMForm:
		vs = append(vs, m.embedderLLMFormView())
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
