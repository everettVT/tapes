package proxy

// Config is the proxy server configuration.
type Config struct {
	// Address to listen on (e.g., ":8080")
	ListenAddr string

	// Upstream LLM provider URL (e.g., "http://localhost:11434")
	UpstreamURL string

	// DBPath is the path to the SQLite database file.
	// Use ":memory:" for an in-memory database, or empty for in-memory.
	DBPath string
}
