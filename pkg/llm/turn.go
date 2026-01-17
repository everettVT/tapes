package llm

// ConversationTurn represents a complete request-response pair for storage in the DAG.
type ConversationTurn struct {
	Request  *ChatRequest  `json:"request"`
	Response *ChatResponse `json:"response"`
}
