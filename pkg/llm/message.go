package llm

// Message represents a single message in a conversation.
type Message struct {
	Role    string   `json:"role"`             // "system", "user", "assistant"
	Content string   `json:"content"`          // The message content
	Images  []string `json:"images,omitempty"` // Optional base64-encoded images (for multimodal)
}
