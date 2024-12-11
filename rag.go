package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/philippgille/chromem-go"
)

type rag struct {
	vectordb *chromem.DB

	convoLLM    llm
	genTitleLLM llm

	embedder embedder

	chats []chat
}

const (
	ragResultsCount         = 20
	ragSimiliarityThreshold = 0.5
	ragNeededCount          = 10

	chunkSize    = 500 // characters per chunk
	chunkOverlap = 50  // overlap between chunks
)

func generateSessionTitle(ctx context.Context, llm llm, chats []chat) (string, error) {
	cs := []chat{
		{
			Role: roleSystem,
			Content: `
Generate ONE line containing ONLY the title. No markdown, no quotes, no explanations.

Rules for the title:
1. EXACTLY 3-6 words
2. NO punctuation marks or special characters
3. NO formatting symbols or markdown
4. Start with action verb or topic noun
5. Use simple everyday words
6. NO technical terms unless absolutely necessary

Examples of good titles:
Building Smart Home Network
Learn Python Programming Basics
Planning Family Summer Vacation

Bad titles (don't do these):
- "Setting up Docker containers" (has quotes)
* Technical Infrastructure Review (has bullet point)
Implementation of ML Models (too technical)
This is a very long title about programming (too many words)
      `,
		},
	}

	cs = append(cs, chats...)

	cs = append(cs, chat{
		Role: roleUser,
		Content: `
	 Based on this conversation, create a clear and concise title that captures its main focus. The title should be immediately understandable to someone new to the discussion.
	     `,
	})

	slog.Info("Gen Title Prompt", "chats", cs)

	res := llm.chat(ctx, cs)
	if res.err != nil {
		return "", res.err
	}

	return res.content, nil
}

func ragSystemPrompt(docs []chromem.Result) string {
	knowledge := ""
	for _, doc := range docs {
		filename := ""
		if name, ok := doc.Metadata["filename"]; ok {
			filename = "[" + name + "]"
		}
		knowledge += "\n---\n" + filename + "\n" + doc.Content + "\n"
	}

	return `
I am an AI assistant who deeply understands and embodies this knowledge:

` + knowledge + `

GUIDELINES:
1. Speak naturally as if this knowledge is your own experience and expertise
2. Never use phrases like "based on documents", "according to", "from the documents", or similar references
3. You can expand the conversation with relevant external knowledge
4. Answer directly and confidently, as if you're sharing your own knowledge
5. Be conversational and engaging

IMPORTANT: Only if you directly quoted or used information from the provided documents, add a new line starting with "Sources: " followed by the filenames. Do not mention sources at all if you didn't use any specific documents.`
}

func chunkDocument(doc chromem.Document) []chromem.Document {
	content := doc.Content
	var chunks []chromem.Document

	if len(content) <= chunkSize {
		return []chromem.Document{doc}
	}

	for i := 0; i < len(content); i += chunkSize - chunkOverlap {
		end := i + chunkSize
		if end > len(content) {
			end = len(content)
		}

		chunk := chromem.Document{
			ID:      fmt.Sprintf("%s-chunk-%d", doc.ID, len(chunks)),
			Content: content[i:end],
			Metadata: map[string]string{
				"filename":   doc.Metadata["filename"],
				"originalID": doc.ID,
				"chunkIndex": fmt.Sprintf("%d", len(chunks)),
			},
		}
		chunks = append(chunks, chunk)

		if end == len(content) {
			break
		}
	}
	return chunks
}

func newRAG(vectordb *chromem.DB, convoLLM, genTitleLLM llm, embedder embedder) *rag {
	return &rag{
		vectordb:    vectordb,
		convoLLM:    convoLLM,
		genTitleLLM: genTitleLLM,
		embedder:    embedder,
	}
}

func (r *rag) clearChats() {
	r.chats = nil
}

func mergeChunks(docs []chromem.Result) []chromem.Result {
	// Group chunks by originalID
	chunkGroups := make(map[string][]chromem.Result)
	for _, doc := range docs {
		originalID := doc.Metadata["originalID"]
		if originalID == "" {
			originalID = doc.ID // Handle non-chunked documents
		}
		chunkGroups[originalID] = append(chunkGroups[originalID], doc)
	}

	// Merge chunks and compute average similarity
	var mergedDocs []chromem.Result
	for _, chunks := range chunkGroups {
		if len(chunks) == 1 {
			mergedDocs = append(mergedDocs, chunks[0])
			continue
		}

		// Sort chunks by chunkIndex
		slices.SortFunc(chunks, func(a, b chromem.Result) int {
			aCI, _ := strconv.Atoi(a.Metadata["chunkIndex"])
			bCI, _ := strconv.Atoi(b.Metadata["chunkIndex"])
			return cmp.Compare(aCI, bCI)
		})

		// Merge content and compute average similarity
		var totalSim float32
		var mergedContent string

		for i, chunk := range chunks {
			totalSim++
			if i == 0 {
				// For first chunk, use it completely
				mergedContent = chunk.Content
				continue
			}

			currentChunkIndex, _ := strconv.Atoi(chunk.Metadata["chunkIndex"])
			previousChunkIndex, _ := strconv.Atoi(chunks[i-1].Metadata["chunkIndex"])
			if previousChunkIndex+1 != currentChunkIndex {
				// If the chunk index is not sequential, skip it
				continue
			}

			// For subsequent chunks, remove the overlapping part
			currentContent := chunk.Content
			if len(currentContent) > chunkOverlap {
				// Skip the first chunkOverlap characters as they're duplicates
				mergedContent += currentContent[chunkOverlap:]
			}
		}

		mergedDocs = append(mergedDocs, chromem.Result{
			ID:         chunks[0].ID,
			Content:    mergedContent,
			Similarity: totalSim / float32(len(chunks)),
			Metadata:   chunks[0].Metadata,
		})
	}
	return mergedDocs
}

func getContextString(chats []chat) string {
	if len(chats) == 0 {
		return ""
	}

	// Get last few message pairs (user + assistant) for context
	// Start from the most recent and work backwards
	contextPairs := 2 // Number of user-assistant pairs to include
	context := ""

	for i := len(chats) - 1; i >= 0 && contextPairs > 0; i-- {
		if chats[i].Role == roleUser {
			context = "User: " + chats[i].Content + "\n" + context
			if i+1 < len(chats) && chats[i+1].Role == roleAssistant {
				context = "Assistant: " + chats[i+1].Content + "\n" + context
				contextPairs--
			}
		}
	}

	return context
}

func (r *rag) chat(ctx context.Context, msg string, index int, documents []document, responses chan<- llmResponseMsg) {
	r.chats = append(r.chats, chat{
		Role:    roleUser,
		Content: msg,
	})

	var ragDocs []chromem.Result

	// Combine current message with context from previous messages
	contextString := getContextString(r.chats[:len(r.chats)-1]) // Exclude current message
	searchText := msg
	if contextString != "" {
		searchText = contextString + "\n" + msg
	}

	for _, doc := range documents {
		rds, err := doc.retrieve(ctx, r.vectordb, searchText, r.embedder.embeddingFunc())
		if err != nil {
			responses <- llmResponseMsg{
				chatIndex: index,
				err:       err,
			}
			return
		}
		ragDocs = append(ragDocs, rds...)
	}

	// First sort by similarity to get the best matches
	slices.SortFunc(ragDocs, func(a, b chromem.Result) int {
		return cmp.Compare(b.Similarity, a.Similarity)
	})

	// Take more results initially to account for merging
	initialCount := ragNeededCount * 2
	if len(ragDocs) > initialCount {
		ragDocs = ragDocs[:initialCount]
	}

	// Merge overlapping chunks
	ragDocs = mergeChunks(ragDocs)

	// Final sort and trim after merging
	slices.SortFunc(ragDocs, func(a, b chromem.Result) int {
		return cmp.Compare(b.Similarity, a.Similarity)
	})

	if len(ragDocs) > ragNeededCount {
		ragDocs = ragDocs[:ragNeededCount]
	}

	ragPrompt := ragSystemPrompt(ragDocs)

	cs := make([]chat, len(r.chats))
	copy(cs, r.chats)

	cs = slices.Insert(cs, 0, chat{
		Role:    roleSystem,
		Content: ragPrompt,
	})

	slog.Info("RAG prompt", "chats", cs)

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
	var chunkedDocs []chromem.Document
	originalFileCount := 0

	for docItem := range documents {
		if ctx.Err() != nil {
			progress <- documentScanLogMsg{
				content: fmt.Sprintf("Error adding documents to collection: %s", ctx.Err()),
				err:     fmt.Errorf("error adding documents to collection: %w", ctx.Err()),
			}
			return
		}

		chunks := chunkDocument(docItem)
		chunkedDocs = append(chunkedDocs, chunks...)
		originalFileCount++

		progress <- documentScanLogMsg{
			content: fmt.Sprintf("Scanning %s (created %d chunks)", docItem.ID, len(chunks)),
		}
	}

	progress <- documentScanLogMsg{
		content: fmt.Sprintf("Scanned %d files into %d chunks, embedding...", originalFileCount, len(chunkedDocs)),
	}

	collName := doc.vectorDBCollectionName()
	docName := doc.Name

	coll, err := r.vectordb.CreateCollection(collName,
		map[string]string{"docName": docName}, r.embedder.embeddingFunc())
	if err != nil {
		progress <- documentScanLogMsg{
			content: fmt.Sprintf("Error creating collection: %s", err),
			err:     fmt.Errorf("error creating collection: %w", err),
		}
		return
	}

	if err := coll.AddDocuments(ctx, chunkedDocs, runtime.NumCPU()); err != nil {
		progress <- documentScanLogMsg{
			content: fmt.Sprintf("Error adding documents to collection: %s", err),
			err:     fmt.Errorf("error adding documents to collection: %w", err),
		}
		return
	}

	progress <- documentScanLogMsg{
		content:          "Embedding complete",
		done:             true,
		scannedFileCount: originalFileCount,
		lastScanTime:     time.Now(),
	}
}

func (m mainModel) refreshRAG() (mainModel, error) {
	if !m.llmIsConfigured() {
		return m, nil
	}

	convo, err := llmFromSetting(m.convoLLMSetting, m.providers)
	if err != nil {
		return mainModel{}, fmt.Errorf("failed to load convo llm: %w", err)
	}

	genTitle, err := llmFromSetting(m.genTitleLLMSetting, m.providers)
	if err != nil {
		return mainModel{}, fmt.Errorf("failed to load title gen llm: %w", err)
	}

	embedder, err := embedderFromSetting(m.embedderLLMSetting, m.providers)
	if err != nil {
		return mainModel{}, fmt.Errorf("failed to load embedder llm: %w", err)
	}

	m.rag = newRAG(m.vectordb, convo, genTitle, embedder)

	return m, nil
}
