// Package llm provides a internal representations of LLM inference API requests
// and responses which are then further mutated and handled.
package llm

// ErrorResponse represents an error from the LLM API.
type ErrorResponse struct {
	Error string `json:"error"`
}
