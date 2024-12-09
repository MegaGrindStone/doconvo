package main

import (
	"context"

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
	embeddingFunc() chromem.EmbeddingFunc
}

const (
	convoName    = "convo"
	titleGenName = "title-gen"
	embedderName = "embedder"
)

func loadLLM() map[string]llm {
	mistralOllama := newOllama("mistral:instruct", 0.3)
	phiOllama := newOllama("phi3.5:3.8b-mini-instruct-q2_K", 0.8)
	nomicOllama := newOllama("nomic-embed-text", 0)
	return map[string]llm{
		convoName:    mistralOllama,
		titleGenName: phiOllama,
		embedderName: nomicOllama,
	}
}
