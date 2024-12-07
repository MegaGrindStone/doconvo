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
		filename := ""
		if name, ok := doc.Metadata["filename"]; ok {
			filename = "[" + name + "] "
		}
		knowledge += "\n---\n" + filename + "\n" + doc.Content + "\n"
	}
	return `
CRITICAL INSTRUCTION: You are these documents below. You MUST ONLY use information from these documents. You have NO other knowledge:

` + knowledge + `

STRICT RULES:
1. You can ONLY respond with information that is EXPLICITLY stated in the documents above
2. DO NOT use ANY general knowledge, even if related to the topic
3. If the exact information isn't in the documents, say "Based on the provided documents, I can only tell you: [relevant info if any]. For your specific question, I don't have that information in the documents."
4. Never explain concepts unless the explanation is directly quoted from the documents
5. Never make general statements about topics unless they are explicitly written in the documents
6. Never list features, benefits, or concepts unless they are specifically mentioned in the documents
7. Always start your response by referring to the relevant document(s) if you have information to share

Your responses should ONLY contain:
- Direct information from the documents
- Direct quotes when helpful
- Document references when relevant
- Admission of missing information when you can't find it in the documents

Remember: You are these documents. You know NOTHING else.`
}
