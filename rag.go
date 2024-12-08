package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/philippgille/chromem-go"
)

type rag struct {
	vectordb *chromem.DB

	convoLLM    llm
	genTitleLLM llm
	embedderLLM llm

	chats []chat
}

func newRAG(vectordb *chromem.DB, convoLLM, genTitleLLM, embedderLLM llm) *rag {
	return &rag{
		vectordb:    vectordb,
		convoLLM:    convoLLM,
		genTitleLLM: genTitleLLM,
		embedderLLM: embedderLLM,
	}
}

func (r *rag) clearChats() {
	r.chats = nil
}

func (r *rag) chat(ctx context.Context, msg string, index int, documents []document, responses chan<- llmResponseMsg) {
	r.chats = append(r.chats, chat{
		Role:    roleUser,
		Content: msg,
	})

	var ragDocs []chromem.Result

	for _, doc := range documents {
		rds, err := doc.retrieve(r.vectordb, msg, r.embedderLLM.embeddingFunc())
		if err != nil {
			responses <- llmResponseMsg{
				chatIndex: index,
				err:       err,
			}
			return
		}
		ragDocs = append(ragDocs, rds...)
	}

	ragPrompt := ragSystemPrompt(ragDocs)

	cs := make([]chat, len(r.chats))
	copy(cs, r.chats)

	cs = slices.Insert(cs, 0, chat{
		Role:    roleSystem,
		Content: ragPrompt,
	})

	res := r.convoLLM.chatStream(ctx, cs)

	newChat := chat{
		Role: roleAssistant,
	}
	for r := range res {
		if r.err != nil {
			responses <- llmResponseMsg{
				chatIndex: index,
				err:       r.err,
			}
			return
		}

		responses <- llmResponseMsg{
			chatIndex:  index,
			content:    r.content,
			isThinking: false,
		}
		newChat.Content += r.content
	}
	r.chats = append(r.chats, newChat)

	responses <- llmResponseMsg{
		done: true,
	}
}

func (r *rag) genTitle() (string, error) {
	title, err := generateSessionTitle(context.Background(), r.genTitleLLM, r.chats)
	if err != nil {
		return "", fmt.Errorf("error generating session title: %w", err)
	}
	if title == "" {
		return "", errors.New("empty title generated")
	}
	return title, nil
}

func (r *rag) scanDocument(ctx context.Context, doc document, progress chan<- documentScanLogMsg) {
	documents := make(chan chromem.Document)

	go r.scanFiles(doc.Path, documents, progress)
	go r.storeDocument(ctx, doc, documents, progress)
}

func (r *rag) scanFiles(path string, documents chan<- chromem.Document, progress chan<- documentScanLogMsg) {
	progress <- documentScanLogMsg{
		content: fmt.Sprintf("Scanning %s", path),
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, runtime.NumCPU())

	if err := filepath.Walk(path, func(path string, f os.FileInfo, err error) error {
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
		progress <- documentScanLogMsg{
			content: fmt.Sprintf("Error scanning %s: %s", path, err),
			err:     err,
		}
		return
	}

	wg.Wait()

	close(documents)
}

func (r *rag) storeDocument(ctx context.Context, doc document, documents <-chan chromem.Document, progress chan<- documentScanLogMsg) {
	docs := make([]chromem.Document, 0)
	for doc := range documents {
		if ctx.Err() != nil {
			progress <- documentScanLogMsg{
				content: fmt.Sprintf("Error adding documents to collection: %s", ctx.Err()),
				err:     fmt.Errorf("error adding documents to collection: %w", ctx.Err()),
			}
			return
		}

		docs = append(docs, doc)

		progress <- documentScanLogMsg{
			content: fmt.Sprintf("Scanning %s", doc.ID),
		}
	}

	progress <- documentScanLogMsg{
		content: fmt.Sprintf("Scanned %d files, embedding...", len(docs)),
	}

	collName := doc.vectorDBCollectionName()
	docName := doc.Name

	coll, err := r.vectordb.CreateCollection(collName,
		map[string]string{"docName": docName}, r.embedderLLM.embeddingFunc())
	if err != nil {
		progress <- documentScanLogMsg{
			content: fmt.Sprintf("Error creating collection: %s", err),
			err:     fmt.Errorf("error creating collection: %w", err),
		}
		return
	}

	if err := coll.AddDocuments(ctx, docs, runtime.NumCPU()); err != nil {
		progress <- documentScanLogMsg{
			content: fmt.Sprintf("Error adding documents to collection: %s", err),
			err:     fmt.Errorf("error adding documents to collection: %w", err),
		}
		return
	}

	progress <- documentScanLogMsg{
		content:          "Embedding complete",
		done:             true,
		scannedFileCount: len(docs),
		lastScanTime:     time.Now(),
	}
}
