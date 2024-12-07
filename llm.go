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
	o := newOllama()
	return map[string]llm{
		convoName:    o,
		titleGenName: o,
		embedderName: newOllawaWithModel("nomic-embed-text"),
	}
}

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

	res := llm.chat(ctx, cs)
	if res.err != nil {
		return "", res.err
	}

	return res.content, nil
}

func ragSystemPrompt(docs []chromem.Result) string {
	knowledge := ""
	for _, doc := range docs {
		knowledge += "\n---\n" + doc.Content + "\n"
	}
	return `
You are a knowledgeable and helpful assistant. You have been provided with additional specific information to enhance your existing knowledge.

Before this conversation, you have thoroughly studied and internalized the following information:

` + knowledge + `

Treat this information as part of your core knowledge base. When answering questions, naturally integrate this information with your general knowledge to provide comprehensive, accurate, and helpful responses. While this information should be your primary source when relevant, there's no need to explicitly mention or distinguish between different sources of information in your responses.

Focus on providing clear, accurate, and helpful answers that best serve the user's needs.`
}
