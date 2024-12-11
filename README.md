# DOConvo

A Terminal User Interface (TUI) application that enables interactive conversations with your documents using Large Language Models (LLM) and Retrieval-Augmented Generation (RAG) techniques.

## Demo

[![asciicast](https://asciinema.org/a/694382.svg)](https://asciinema.org/a/694382)

The demo above showcases a conversation with my DevOps journey notes, where the AI assistant embodies the knowledge from the documents to provide contextual responses.

The demo uses the following LLM configuration:

| Role | LLM Provider and Model |
|---------|-------|
| Convo | `Anthropic:claude-3-5-sonnet-20241022` |
| Generate Title | `OpenAI:gpt-4` |
| Embedder | `Ollama:nomic-embed-text` |

## Features

- Interactive TUI for natural conversations with your documents
- RAG-powered responses using your document knowledge base
- Support for multiple LLM providers (Ollama, Anthropic, OpenAI)
- Contextual understanding and relevant answers

## Installation

### Install via go

`go install github.com/MegaGrindStone/doconvo@latest`

## Usage

### Initial Setup

1. Launch DOConvo by running:
   ```shell
   doconvo
   ```

2. On first launch, DOConvo will create a configuration directory:
   - Linux/macOS: `~/.config/doconvo/`
   - Windows: `%AppData%\doconvo\`
   This directory stores your settings, embedded documents, and log files.

3. First-time run will open the `Options` screen where you need to:
   - Configure LLM Providers (at least one)
   - Set up required Roles (Convo, Generate Title, and Embedder)

### Document Embedding

- While optional, embedding documents is recommended for meaningful conversations
- To embed documents:
  1. Navigate to document embedding options (available after Embedder LLM setup)
  2. Select directories containing your documents
  3. All files in selected directories and subdirectories will be processed (`.git` directories are ignored)
  4. Multiple document directories can be embedded

### Starting Conversations

1. Return to the main screen
2. Create a new conversation session
3. Start interacting with your documents through natural language queries

The assistant will use the embedded documents as context to provide relevant responses based on your document content.

## Configuration

### Accessing Configuration

Press `ctrl+o` from the main screen to access the Options menu where you can configure LLM providers and roles.

### Supported LLM Providers

DOConvo supports the following LLM providers:

- [Ollama](https://ollama.com/)
  - Required parameter: `Host`
  - Default value: Uses `OLLAMA_HOST` environment variable
- [Anthropic](https://www.anthropic.com/)
  - Required parameter: `API Key`
  - Default value: Uses `ANTHROPIC_API_KEY` environment variable
- [OpenAI](https://openai.com/)
  - Required parameter: `API Key`
  - Default value: Uses `OPENAI_API_KEY` environment variable

### Required LLM Roles

The application requires three LLM roles to be configured:

1. **Convo LLM**: Handles the main conversation interactions
2. **Generate Title LLM**: Creates titles for chat sessions
3. **Embedder LLM**: Processes document embeddings

You can freely mix and match different LLM providers and their available models for each role based on your preferences and requirements.

## Limitations

### File Type Support
- Currently supports only text-based files
- Image files are not processed or understood
- PDF support is limited:
  - Simple PDF files may work
  - Complex PDFs with mixed content may produce unreliable results

### Source Code Handling
- Source code files are processed as plain text
- Code without sufficient comments may result in:
  - Poor context understanding
  - Less accurate responses
  - Potential confusion in conversations

### Document Processing
- All files in selected directories (and subdirectories) are processed
- No selective file processing - entire directories are embedded
- Large directories with many files may require significant processing time

## Troubleshooting

### Logging
- DOConvo automatically maintains log files in the configuration directory
- All application errors are recorded in these logs
- Log files can help diagnose issues and track application behavior

### Debug Mode
- Launch with debug mode: `doconvo -debug`
- Debug mode provides additional information:
  - LLM prompts and responses
  - Detailed error traces
  - System operation logs

### Common Issues
- If LLM connections fail:
  - Verify API keys are correctly set
  - Check network connectivity
  - Ensure LLM provider services are available
- For embedding issues:
  - Confirm file permissions
  - Check available disk space
  - Verify file format compatibility

## Acknowledgements

This project stands on the shoulders of these excellent open-source projects:

- [Bubble Tea Framework](https://github.com/charmbracelet/bubbletea)
  - Powerful Go framework for terminal user interfaces
  - Enables smooth and responsive TUI implementation
  - Makes terminal applications development a joy

- [BBolt](https://github.com/etcd-io/bbolt)
  - Reliable embedded key-value database
  - Simple yet powerful storage solution
  - Perfect for application data persistence

- [Chromem-Go](https://github.com/philippgille/chromem-go)
  - Efficient embeddable vector database
  - Inspired the creation of this project
  - Sparked the idea to build a RAG-based conversation tool

- [CLI for ChatGPT](https://github.com/j178/chatgpt)
  - Influenced the chat interface design
  - Provided insights for conversation flow
  - Inspired many UX improvements
