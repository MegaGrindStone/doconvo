package main

import "context"

type aiResponse struct {
	content string
	err     error
}

type aiResponseMsg struct {
	chatIndex  int
	content    string
	isThinking bool
	err        error
	done       bool
}

type aiResponseTitleMsg struct {
	title        string
	sessionIndex int
	err          error
}

type ai interface {
	chat(context.Context, []chat) aiResponse
	chatStream(context.Context, []chat) <-chan aiResponse
}

const (
	convoName    = "convo"
	titleGenName = "title-gen"
)

func loadAI() map[string]ai {
	o := newOllama()
	return map[string]ai{
		convoName:    o,
		titleGenName: o,
	}
}

func generateSessionTitle(ctx context.Context, ai ai, chats []chat) (string, error) {
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

	res := ai.chat(ctx, cs)
	if res.err != nil {
		return "", res.err
	}

	return res.content, nil
}
