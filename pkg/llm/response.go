package llm

import "time"

// ChatResponse represents a chat completion response (Ollama-compatible).
type ChatResponse struct {
	Model     string    `json:"model"`      // Model that generated the response
	CreatedAt time.Time `json:"created_at"` // Response timestamp
	Message   Message   `json:"message"`    // The assistant's response
	Done      bool      `json:"done"`       // Whether generation is complete

	// Metrics (only present when done=true)
	TotalDuration      int64 `json:"total_duration,omitempty"`       // Total time in nanoseconds
	LoadDuration       int64 `json:"load_duration,omitempty"`        // Model load time
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`    // Tokens in prompt
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"` // Prompt processing time
	EvalCount          int   `json:"eval_count,omitempty"`           // Generated tokens
	EvalDuration       int64 `json:"eval_duration,omitempty"`        // Generation time

	// Context for continuation (Ollama-specific)
	Context []int `json:"context,omitempty"` // Token context for follow-up requests
}
