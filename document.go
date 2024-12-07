package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/philippgille/chromem-go"
)

type document struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	Path             string    `json:"path"`
	ScannedFileCount int       `json:"scannedFileCount"`
	LastScanTime     time.Time `json:"lastScanTime"`
}

type documentScanLogMsg struct {
	content string
	err     error

	done bool
}

const (
	ragResultsCount         = 10
	ragSimiliarityThreshold = 0.35
)

func (m mainModel) initDocuments() (mainModel, error) {
	var err error
	m.documents, err = loadDocuments(m.db)
	if err != nil {
		return mainModel{}, fmt.Errorf("failed to load documents: %w", err)
	}

	items := make([]list.Item, len(m.documents))
	for i, item := range m.documents {
		items[i] = item
	}

	m.documentsList = defaultList("Documents List", m.keymap, func() []key.Binding {
		return []key.Binding{
			m.keymap.new,
			m.keymap.escape,
		}
	}, func() []key.Binding {
		return []key.Binding{
			m.keymap.new,
			m.keymap.delete,
			m.keymap.pick,
			m.keymap.escape,
		}
	})
	m.documentsList.SetItems(items)

	return m, nil
}

func (m mainModel) updateDocumentsSize() mainModel {
	height := m.height - logoHeight()

	if m.err != nil {
		height -= errHeight(m.width, m.err)
	}

	m.documentsList.SetSize(m.width, height)
	return m
}

func (m mainModel) handleDocumentsEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.updateDocumentsSize()
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.escape):
			return m.setViewState(viewStateOptions), nil
		case key.Matches(msg, m.keymap.new):
			return m.newDocument()
		case key.Matches(msg, m.keymap.pick):
			return m.selectDocument(m.documentsList.Index())
		case key.Matches(msg, m.keymap.delete):
			return m.deleteDocument(m.documentsList.Index()), nil
		}
	}

	var cmd tea.Cmd
	m.documentsList, cmd = m.documentsList.Update(msg)
	return m, cmd
}

func (m mainModel) documentsView() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		logoView(),
		m.documentsList.View(),
	)
}

func (m mainModel) newDocument() (mainModel, tea.Cmd) {
	newDocument := document{
		ScannedFileCount: 0,
		LastScanTime:     time.Now(),
	}
	if err := saveDocument(m.db, &newDocument); err != nil {
		m.err = fmt.Errorf("error creating new document: %w", err)
		return m.updateDocumentsSize(), nil
	}
	m.documents = append(m.documents, newDocument)
	newIndex := len(m.documents) - 1

	var cmds []tea.Cmd

	// If we directly return this InsertItem command, the document list will not be
	// updated. This is because the updated list won't be picked up by the copy
	// of the model returned by the m.selectDocument below, that's why we need to
	// make sure this command is executed and updated in the main model.
	cmd := m.documentsList.InsertItem(newIndex, newDocument)
	cmds = append(cmds, cmd)

	m, cmd = m.selectDocument(newIndex)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m mainModel) selectDocument(index int) (mainModel, tea.Cmd) {
	m.selectedDocumentIndex = index

	return m.setViewState(viewStateDocumentForm).
		updateDocumentFormSize().
		newDocumentForm()
}

func (m mainModel) deleteDocument(index int) mainModel {
	document := m.documents[index]

	if err := deleteDocument(m.db, document.ID); err != nil {
		m.err = fmt.Errorf("error deleting document: %w", err)
		return m.updateDocumentsSize()
	}

	m.documents = slices.Delete(m.documents, index, index+1)
	m.documentsList.RemoveItem(index)

	return m
}

func (m mainModel) newDocumentForm() (mainModel, tea.Cmd) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		m.err = fmt.Errorf("error getting user home directory: %w", err)
		return m, nil
	}

	selectedDocument := m.documents[m.selectedDocumentIndex]
	if selectedDocument.Path == "" {
		selectedDocument.Path = homeDir
	}
	name := selectedDocument.Name
	path := selectedDocument.Path

	m.documentForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("documentName").
				Title("Document Name").
				Description("Enter the name of the document.").
				Placeholder("Document Name").
				Value(&name),
			newFormFilePicker(huh.NewFilePicker().
				Key("documentPath").
				Title("Document Path").
				Description("Select the path of the document.").
				FileAllowed(false).
				DirAllowed(true).
				CurrentDirectory(selectedDocument.Path).
				Value(&path),
				m.keymap.formKeymap.FilePicker),
			huh.NewConfirm().
				Key("documentConfirm").
				Title("Scan").
				Description("Are you sure you want to scan this document?").
				Affirmative("Yes").
				Negative("Back"),
		),
	).
		WithWidth(m.documentFormWidth).
		WithHeight(m.documentFormHeight).
		WithKeyMap(m.keymap.formKeymap).
		WithShowErrors(true).
		WithShowHelp(true)

	return m, m.documentForm.PrevField()
}

func (m mainModel) updateDocumentFormSize() mainModel {
	titleHeight := lipgloss.Height(titleStyle.Render(""))
	height := m.height - logoHeight() - titleHeight

	if m.err != nil {
		height -= errHeight(m.width, m.err)
	}

	m.documentFormWidth = m.width
	m.documentFormHeight = height

	return m
}

func (m mainModel) handleDocumentFormEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.updateDocumentFormSize()
	case tea.KeyMsg:
		if key.Matches(msg, m.keymap.escape) {
			return m.setViewState(viewStateDocuments), nil
		}
	}

	form, cmd := m.documentForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.documentForm = f
	}

	if m.documentForm.State != huh.StateCompleted {
		return m, cmd
	}

	if !m.documentForm.GetBool("documentConfirm") {
		return m.setViewState(viewStateDocuments), nil
	}

	selectedDocument := m.documents[m.selectedDocumentIndex]
	selectedDocument.Name = m.documentForm.GetString("documentName")
	selectedDocument.Path = m.documentForm.GetString("documentPath")

	if err := saveDocument(m.db, &selectedDocument); err != nil {
		m.err = fmt.Errorf("error creating new document: %w", err)
		return m.updateDocumentFormSize(), nil
	}

	m.documents[m.selectedDocumentIndex] = selectedDocument
	m.documentsList.SetItem(m.selectedDocumentIndex, selectedDocument)

	return m.setViewState(viewStateDocumentScan).scanDocument(), nil
}

func (m mainModel) documentFormView() string {
	selectedDocument := m.documents[m.selectedDocumentIndex]
	title := selectedDocument.Name
	if selectedDocument.Name == "" {
		title = "New Document"
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		logoView(),
		titleStyle.Render(title),
		m.documentForm.View(),
	)
}

func (m mainModel) initDocumentScan() mainModel {
	m.documentScanViewport = viewport.New(0, 0)
	m.documentScanViewport.KeyMap = m.keymap.viewportKeymap

	m.documentScanProgress = make(chan documentScanLogMsg)

	return m
}

func (m mainModel) updateDocumentScanSize() mainModel {
	titleHeight := lipgloss.Height(titleStyle.Render(""))
	helpHeight := lipgloss.Height(m.helpModel.View(m.keymap))
	height := m.height - logoHeight() - titleHeight - helpHeight

	if m.err != nil {
		height -= errHeight(m.width, m.err)
	}

	m.documentScanViewport.Width = m.width
	m.documentScanViewport.Height = height

	m.documentScanViewport.SetContent(strings.Join(m.documentScanLogs, "\n"))
	m.documentScanViewport.GotoBottom()

	return m
}

func (m mainModel) handleDocumentScanEvents(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.updateDocumentScanSize()
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.escape):
			if m.documentScanCancelFunc != nil {
				m.documentScanCancelFunc()
				m.documentScanCancelFunc = nil
				return m, nil
			}

			m.err = nil
			return m.setViewState(viewStateDocuments).updateDocumentsSize(), nil
		case key.Matches(msg, m.keymap.openHelp):
			m.keymap.openHelp.SetEnabled(false)
			m.keymap.closeHelp.SetEnabled(true)
			m.helpModel.ShowAll = true
			return m.updateDocumentScanSize(), nil
		case key.Matches(msg, m.keymap.closeHelp):
			m.keymap.closeHelp.SetEnabled(false)
			m.keymap.openHelp.SetEnabled(true)
			m.helpModel.ShowAll = false
			return m.updateDocumentScanSize(), nil
		}
	case documentScanLogMsg:
		return m.handleScanLogMsg(msg), nil
	}

	var cmd tea.Cmd
	m.documentScanViewport, cmd = m.documentScanViewport.Update(msg)
	return m, cmd
}

func (m mainModel) documentScanView() string {
	selectedDocument := m.documents[m.selectedDocumentIndex]
	return lipgloss.JoinVertical(lipgloss.Left,
		logoView(),
		titleStyle.Render(fmt.Sprintf("Scanning %s", selectedDocument.Path)),
		m.documentScanViewport.View(),
		m.helpModel.View(m.keymap),
	)
}

func (m mainModel) handleScanLogMsg(msg documentScanLogMsg) mainModel {
	m.documentScanLogs = append(m.documentScanLogs, msg.content)

	if msg.err != nil {
		m.err = msg.err
		m.documentScanCancelFunc = nil

		m.documentScanViewport.SetContent(strings.Join(m.documentScanLogs, "\n"))
		m.documentScanViewport.GotoBottom()

		return m
	}

	if msg.done {
		m.documentScanLogs = append(m.documentScanLogs,
			fmt.Sprintf("Scan complete in %s", time.Since(m.documentScanStartTime)))
		m.documentsList.SetItem(m.selectedDocumentIndex, m.documents[m.selectedDocumentIndex])
		m.documentScanCancelFunc = nil
	}

	m.documentScanViewport.SetContent(strings.Join(m.documentScanLogs, "\n"))
	m.documentScanViewport.GotoBottom()

	return m
}

func (m mainModel) scanDocument() mainModel {
	path := m.documents[m.selectedDocumentIndex].Path

	m.err = nil
	m.documentScanStartTime = time.Now()
	m.documentScanLogs = make([]string, 0)

	ctx, cancel := context.WithCancel(context.Background())
	m.documentScanCancelFunc = cancel

	documents := make(chan chromem.Document)

	go func(p string) {
		m.documentScanProgress <- documentScanLogMsg{
			content: fmt.Sprintf("Scanning %s", p),
		}

		var wg sync.WaitGroup
		semaphore := make(chan struct{}, runtime.NumCPU())

		if err := filepath.Walk(p, func(path string, f os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip git directories
			if f.IsDir() && f.Name() == ".git" {
				return filepath.SkipDir
			}

			if f.IsDir() {
				return nil
			}

			wg.Add(1)
			go func(p string) {
				semaphore <- struct{}{}
				defer func() {
					<-semaphore
					wg.Done()
				}()

				fileData, err := os.ReadFile(p)
				if err != nil {
					return
				}

				// Avoid processing empty files
				if len(fileData) == 0 {
					return
				}

				documents <- chromem.Document{
					ID:      p,
					Content: string(fileData),
					Metadata: map[string]string{
						"filename": filepath.Base(path),
					},
				}
			}(path)

			return nil
		}); err != nil {
			m.documentScanProgress <- documentScanLogMsg{
				content: fmt.Sprintf("Error scanning %s: %s", p, err),
				err:     err,
			}
			return
		}

		wg.Wait()

		close(documents)
	}(path)

	go func() {
		docs := make([]chromem.Document, 0)
		for doc := range documents {
			if ctx.Err() != nil {
				m.documentScanProgress <- documentScanLogMsg{
					content: fmt.Sprintf("Error adding documents to collection: %s", ctx.Err()),
					err:     fmt.Errorf("error adding documents to collection: %w", ctx.Err()),
				}
				return
			}

			docs = append(docs, doc)

			m.documentScanProgress <- documentScanLogMsg{
				content: fmt.Sprintf("Scanning %s", doc.ID),
			}
		}

		m.documentScanProgress <- documentScanLogMsg{
			content: fmt.Sprintf("Scanned %d files, embedding...", len(docs)),
		}

		m.documents[m.selectedDocumentIndex].ScannedFileCount = len(docs)
		m.documents[m.selectedDocumentIndex].LastScanTime = time.Now()
		doc := m.documents[m.selectedDocumentIndex]
		if err := saveDocument(m.db, &doc); err != nil {
			m.documentScanProgress <- documentScanLogMsg{
				content: fmt.Sprintf("Error saving knowledge: %s", err),
				err:     fmt.Errorf("error saving knowledge: %w", err),
			}
			return
		}

		collName := doc.vectorDBCollectionName()
		docName := doc.Name

		coll, err := m.vectordb.CreateCollection(collName,
			map[string]string{"docName": docName}, m.embedderLLM.embeddingFunc())
		if err != nil {
			m.documentScanProgress <- documentScanLogMsg{
				content: fmt.Sprintf("Error creating collection: %s", err),
				err:     fmt.Errorf("error creating collection: %w", err),
			}
			return
		}

		if err := coll.AddDocuments(ctx, docs, runtime.NumCPU()); err != nil {
			m.documentScanProgress <- documentScanLogMsg{
				content: fmt.Sprintf("Error adding documents to collection: %s", err),
				err:     fmt.Errorf("error adding documents to collection: %w", err),
			}
			return
		}

		m.documentScanProgress <- documentScanLogMsg{
			content: "Embedding complete",
			done:    true,
		}
	}()

	return m.updateDocumentScanSize()
}

func (d document) Title() string {
	path := "path not set"
	if d.Path != "" {
		path = d.Path
	}
	return fmt.Sprintf("%s (%s)", d.Name, path)
}

func (d document) Description() string {
	lst := "Not scanned yet"
	if !d.LastScanTime.IsZero() {
		lst = fmt.Sprintf("Last scan time: %s", d.LastScanTime.Format(time.RFC1123))
	}
	return fmt.Sprintf("File count: %d; %s", d.ScannedFileCount, lst)
}

func (d document) FilterValue() string {
	return d.Name
}

func (d document) vectorDBCollectionName() string {
	return fmt.Sprintf("doc-%d", d.ID)
}

func (d document) retrieve(vectordb *chromem.DB, key string, embedFunc chromem.EmbeddingFunc) ([]chromem.Result, error) {
	var res []chromem.Result

	collName := d.vectorDBCollectionName()
	coll := vectordb.GetCollection(collName, embedFunc)
	if coll == nil {
		return nil, fmt.Errorf("failed to get vectordb collection %s", collName)
	}
	docRes, err := coll.Query(context.Background(), key, ragResultsCount, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query vectordb collection %s: %w", collName, err)
	}
	for _, r := range docRes {
		if r.Similarity >= ragSimiliarityThreshold {
			res = append(res, r)
		}
	}

	return res, nil
}
