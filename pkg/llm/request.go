package llm

// ChatRequest represents a chat completion request (Ollama-compatible).
type ChatRequest struct {
	Model    string    `json:"model"`            // Model name (e.g., "llama2", "mistral")
	Messages []Message `json:"messages"`         // Conversation history
	Stream   *bool     `json:"stream,omitempty"` // Whether to stream responses (default: true in Ollama)
	Format   string    `json:"format,omitempty"` // Response format ("json" for JSON mode)

	// Generation options
	Options *Options `json:"options,omitempty"`

	// Keep model loaded
	KeepAlive string `json:"keep_alive,omitempty"` // How long to keep model in memory
}
