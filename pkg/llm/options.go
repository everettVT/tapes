package llm

// Options contains model inference parameters.
type Options struct {
	// Sampling parameters
	Temperature *float64 `json:"temperature,omitempty"` // Creativity (0.0-2.0)
	TopP        *float64 `json:"top_p,omitempty"`       // Nucleus sampling threshold
	TopK        *int     `json:"top_k,omitempty"`       // Top-k sampling
	Seed        *int     `json:"seed,omitempty"`        // Random seed for reproducibility

	// Length parameters
	NumPredict *int `json:"num_predict,omitempty"` // Max tokens to generate
	NumCtx     *int `json:"num_ctx,omitempty"`     // Context window size

	// Repetition control
	RepeatPenalty *float64 `json:"repeat_penalty,omitempty"` // Penalty for repeating tokens
	RepeatLastN   *int     `json:"repeat_last_n,omitempty"`  // Tokens to consider for penalty

	// Stop sequences
	Stop []string `json:"stop,omitempty"` // Stop generation at these sequences
}
