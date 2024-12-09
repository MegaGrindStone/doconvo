package main

import (
	"context"
	"os"

	"github.com/philippgille/chromem-go"
)

type llmResponse struct {
	content string
	err     error
}

type llmResponseMsg struct {
	chatIndex  int
	content    string
	isThinking bool
	err        error
	done       bool
}

type llmResponseTitleMsg struct {
	title        string
	sessionIndex int
	err          error
}

type llm interface {
	chat(context.Context, []chat) llmResponse
	chatStream(context.Context, []chat) <-chan llmResponse
}

type embedder interface {
	embeddingFunc() chromem.EmbeddingFunc
}

const (
	convoName    = "convo"
	titleGenName = "title-gen"
	embedderName = "embedder"
)

func loadLLM() map[string]llm {
	sonnet := newAnthropic(os.Getenv("ANTHROPIC_API_KEY"), "claude-3-5-sonnet-20241022", 0.8)
	haiku := newAnthropic(os.Getenv("ANTHROPIC_API_KEY"), "claude-3-haiku-20240307", 0.2)
	return map[string]llm{
		convoName:    sonnet,
		titleGenName: haiku,
	}
}

func loadEmbedder() embedder {
	return newOllama("nomic-embed-text", 0)
}

func extractSystemChat(chats []chat) (string, []chat) {
	if len(chats) == 0 {
		return "", chats
	}

	if chats[0].Role == roleSystem {
		return chats[0].Content, chats[1:]
	}

	return "", chats
}
