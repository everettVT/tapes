// Package proxy provides an LLM inference proxy that stores conversations in a Merkle DAG.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"

	"github.com/papercomputeco/tapes/pkg/llm"
	"github.com/papercomputeco/tapes/pkg/merkle"
)

// Proxy is a client, LLM inference proxy that instruments storing sessions as Merkle DAGs.
// The proxy is designed to be stateless as it builds nodes from incoming messages
// and stores them in a content-addressable merkle.Storer.
type Proxy struct {
	config     Config
	storer     merkle.Storer
	logger     *zap.Logger
	httpClient *http.Client
	server     *fiber.App
}

// New creates a new Proxy.
func New(config Config, logger *zap.Logger) (*Proxy, error) {
	var storer merkle.Storer
	var err error

	if config.DBPath != "" {
		storer, err = merkle.NewSQLiteStorer(config.DBPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create SQLite storer: %w", err)
		}
		logger.Info("using SQLite storage", zap.String("path", config.DBPath))
	} else {
		storer = merkle.NewMemoryStorer()
		logger.Info("using in-memory storage")
	}

	app := fiber.New(fiber.Config{
		// Disable startup message for cleaner logs
		DisableStartupMessage: true,
		// Enable streaming
		StreamRequestBody: true,
	})

	p := &Proxy{
		config: config,
		storer: storer,
		logger: logger,
		server: app,
		httpClient: &http.Client{
			// LLM requests can be slow, especially with thinking blocks
			Timeout: 5 * time.Minute,
		},
	}

	// Register routes
	app.Post("/api/chat", p.handleChat)

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(map[string]string{"status": "ok"})
	})

	// DAG inspection endpoints
	app.Get("/dag/stats", p.handleDAGStats)
	app.Get("/dag/node/:hash", p.handleGetNode)
	app.Get("/dag/history", p.handleListHistories)
	app.Get("/dag/history/:hash", p.handleGetHistory)

	return p, nil
}

// Run starts the proxy server on the given listening address
func (p *Proxy) Run() error {
	p.logger.Info("starting proxy server",
		zap.String("listen", p.config.ListenAddr),
		zap.String("upstream", p.config.UpstreamURL),
	)

	return p.server.Listen(p.config.ListenAddr)
}

// Close shuts down the proxy and releases resources.
func (p *Proxy) Close() error {
	return p.storer.Close()
}

// handleChat proxies chat requests to the upstream LLM and stores the conversation.
// The proxy is transparent - it forwards requests to the upstream LLM and stores
// the conversation in the Merkle DAG. Content-addressability means:
// - Identical message histories automatically deduplicate (same hashes)
// - Different responses from the LLM create branches from the common ancestor
// - No session IDs or special headers needed - the content IS the identity
func (p *Proxy) handleChat(c *fiber.Ctx) error {
	startTime := time.Now()

	// Parse the incoming request
	var req llm.ChatRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		p.logger.Error("failed to parse request", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(llm.ErrorResponse{Error: "invalid request body"})
	}

	p.logger.Debug("received chat request",
		zap.String("model", req.Model),
		zap.Int("message_count", len(req.Messages)),
		zap.Bool("stream", req.Stream != nil && *req.Stream),
	)

	// Determine if streaming
	streaming := req.Stream == nil || *req.Stream // Ollama defaults to streaming

	if streaming {
		return p.handleStreamingChat(c, &req, startTime)
	} else {
		return p.handleNonStreamingChat(c, &req, startTime)
	}
}

// handleNonStreamingChat handles non-streaming chat requests.
func (p *Proxy) handleNonStreamingChat(c *fiber.Ctx, req *llm.ChatRequest, startTime time.Time) error {
	// Forward to upstream
	resp, err := p.forwardRequest(req)
	if err != nil {
		p.logger.Error("failed to forward request", zap.Error(err))
		return c.Status(fiber.StatusBadGateway).JSON(llm.ErrorResponse{Error: "upstream request failed"})
	}

	p.logger.Debug("received response from upstream",
		zap.String("model", resp.Model),
		zap.String("role", resp.Message.Role),
		zap.String("content_preview", truncate(resp.Message.Content, 100)),
		zap.Duration("duration", time.Since(startTime)),
	)

	// Store in DAG - content-addressability handles deduplication automatically
	headHash, err := p.storeConversationTurn(c.Context(), req, resp)
	if err != nil {
		p.logger.Error("failed to store conversation", zap.Error(err))
		// Continue - don't fail the request just because storage failed
	} else {
		p.logger.Info("conversation stored", zap.String("head_hash", truncate(headHash, 16)))
	}

	// Return response to client
	return c.JSON(resp)
}

// handleStreamingChat handles streaming chat requests.
func (p *Proxy) handleStreamingChat(c *fiber.Ctx, req *llm.ChatRequest, startTime time.Time) error {
	// Build upstream request
	reqBody, err := json.Marshal(req)
	if err != nil {
		p.logger.Error("failed to marshal request", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(llm.ErrorResponse{Error: "internal error"})
	}

	upstreamURL := p.config.UpstreamURL + "/api/chat"
	httpReq, err := http.NewRequestWithContext(c.Context(), "POST", upstreamURL, bytes.NewReader(reqBody))
	if err != nil {
		p.logger.Error("failed to create upstream request", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(llm.ErrorResponse{Error: "internal error"})
	}
	httpReq.Header.Set("Content-Type", "application/json")

	p.logger.Debug("forwarding streaming request to upstream",
		zap.String("url", upstreamURL),
	)

	// Make the request
	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.logger.Error("upstream request failed", zap.Error(err))
		return c.Status(fiber.StatusBadGateway).JSON(llm.ErrorResponse{Error: "upstream request failed"})
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		p.logger.Error("upstream returned error",
			zap.Int("status", httpResp.StatusCode),
			zap.String("body", string(body)),
		)
		return c.Status(httpResp.StatusCode).JSON(llm.ErrorResponse{Error: "upstream error"})
	}

	// Set up streaming response headers
	c.Set("Content-Type", "application/x-ndjson")
	c.Set("Transfer-Encoding", "chunked")

	// Use Fiber's streaming response with proper bufio.Writer signature
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		// Stream chunks and accumulate the full response
		var fullContent strings.Builder
		var finalResp *llm.ChatResponse

		scanner := bufio.NewScanner(httpResp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var chunk llm.StreamChunk
			if err := json.Unmarshal(line, &chunk); err != nil {
				p.logger.Warn("failed to parse chunk", zap.Error(err), zap.String("line", string(line)))
				continue
			}

			// Accumulate content
			fullContent.WriteString(chunk.Message.Content)

			p.logger.Debug("streaming chunk",
				zap.Bool("done", chunk.Done),
				zap.String("content", truncate(chunk.Message.Content, 50)),
			)

			// Write chunk to client
			w.Write(line)
			w.Write([]byte("\n"))
			w.Flush()

			// Capture final response
			if chunk.Done {
				finalResp = &llm.ChatResponse{
					Model:              chunk.Model,
					CreatedAt:          chunk.CreatedAt,
					Message:            llm.Message{Role: "assistant", Content: fullContent.String()},
					Done:               true,
					TotalDuration:      chunk.TotalDuration,
					LoadDuration:       chunk.LoadDuration,
					PromptEvalCount:    chunk.PromptEvalCount,
					PromptEvalDuration: chunk.PromptEvalDuration,
					EvalCount:          chunk.EvalCount,
					EvalDuration:       chunk.EvalDuration,
				}
			}
		}

		if err := scanner.Err(); err != nil {
			p.logger.Error("error reading stream", zap.Error(err))
		}

		// Store the complete conversation turn
		if finalResp != nil {
			p.logger.Debug("streaming complete",
				zap.String("full_content_preview", truncate(fullContent.String(), 200)),
				zap.Duration("duration", time.Since(startTime)),
			)
			headHash, err := p.storeConversationTurn(context.Background(), req, finalResp)
			if err != nil {
				p.logger.Error("failed to store conversation", zap.Error(err))
			} else {
				p.logger.Info("conversation stored", zap.String("head_hash", truncate(headHash, 16)))
			}
		}
	}))

	return nil
}

// forwardRequest forwards a non-streaming request to the upstream LLM.
func (p *Proxy) forwardRequest(req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Ensure non-streaming
	streaming := false
	req.Stream = &streaming

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	upstreamURL := p.config.UpstreamURL + "/api/chat"
	p.logger.Debug("forwarding request to upstream",
		zap.String("url", upstreamURL),
		zap.Int("body_size", len(reqBody)),
	)

	httpReq, err := http.NewRequest("POST", upstreamURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned %d: %s", httpResp.StatusCode, string(body))
	}

	var resp llm.ChatResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}

// storeConversationTurn stores a request-response pair in the Merkle DAG.
// It builds nodes for each message, linking them together, and returns the
// hash of the final (head) node. The content-addressable nature of the DAG
// means identical conversations will deduplicate automatically, and divergent
// conversations will branch from their common ancestor.
//
// Each message in the request becomes a node. The first message is a root node
// (no parent), and subsequent messages link to the previous. If the same message
// sequence was stored before, the hashes will match and no new nodes are created
// (deduplication). When the LLM returns a different response, only the response
// node differs, creating a branch from the shared conversation prefix.
func (p *Proxy) storeConversationTurn(ctx context.Context, req *llm.ChatRequest, resp *llm.ChatResponse) (string, error) {
	var parent *merkle.Node

	// Store each message from the request as nodes
	// These represent the conversation history - if the same history was sent before,
	// the nodes will already exist (deduplication via content-addressing)
	for _, msg := range req.Messages {
		content := map[string]any{
			"type":    "message",
			"role":    msg.Role,
			"content": msg.Content,
			"model":   req.Model,
		}

		node := merkle.NewNode(content, parent)
		if err := p.storer.Put(ctx, node); err != nil {
			return "", fmt.Errorf("storing message node: %w", err)
		}

		p.logger.Debug("stored message in DAG",
			zap.String("hash", truncate(node.Hash, 16)),
			zap.String("role", msg.Role),
			zap.String("content_preview", truncate(msg.Content, 50)),
		)

		parent = node
	}

	// Store the response message
	responseContent := map[string]any{
		"type":    "message",
		"role":    resp.Message.Role,
		"content": resp.Message.Content,
		"model":   resp.Model,
		"metrics": map[string]any{
			"total_duration_ns":       resp.TotalDuration,
			"prompt_eval_count":       resp.PromptEvalCount,
			"prompt_eval_duration_ns": resp.PromptEvalDuration,
			"eval_count":              resp.EvalCount,
			"eval_duration_ns":        resp.EvalDuration,
		},
	}

	responseNode := merkle.NewNode(responseContent, parent)
	if err := p.storer.Put(ctx, responseNode); err != nil {
		return "", fmt.Errorf("storing response node: %w", err)
	}

	p.logger.Debug("stored response in DAG",
		zap.String("hash", truncate(responseNode.Hash, 16)),
		zap.String("content_preview", truncate(resp.Message.Content, 50)),
	)

	return responseNode.Hash, nil
}

// handleDAGStats returns statistics about the DAG.
func (p *Proxy) handleDAGStats(c *fiber.Ctx) error {
	ctx := c.Context()

	nodes, err := p.storer.List(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(llm.ErrorResponse{Error: "failed to list nodes"})
	}

	roots, err := p.storer.Roots(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(llm.ErrorResponse{Error: "failed to get roots"})
	}

	leaves, err := p.storer.Leaves(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(llm.ErrorResponse{Error: "failed to get leaves"})
	}

	stats := map[string]any{
		"total_nodes": len(nodes),
		"root_count":  len(roots),
		"leaf_count":  len(leaves),
	}

	return c.JSON(stats)
}

// handleGetNode returns a single node by its hash.
func (p *Proxy) handleGetNode(c *fiber.Ctx) error {
	hash := c.Params("hash")
	if hash == "" {
		return c.Status(fiber.StatusBadRequest).JSON(llm.ErrorResponse{Error: "hash parameter required"})
	}

	node, err := p.storer.Get(c.Context(), hash)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(llm.ErrorResponse{Error: "node not found"})
	}

	return c.JSON(node)
}

// HistoryResponse contains the conversation history for a given node.
type HistoryResponse struct {
	// Messages in chronological order (oldest first, up to and including the requested node)
	Messages []HistoryMessage `json:"messages"`
	// HeadHash is the hash of the node that was requested
	HeadHash string `json:"head_hash"`
	// Depth is the number of messages in the history
	Depth int `json:"depth"`
}

// HistoryMessage represents a message in the conversation history.
type HistoryMessage struct {
	Hash       string         `json:"hash"`
	ParentHash *string        `json:"parent_hash,omitempty"`
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	Model      string         `json:"model,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// handleListHistories returns all conversation histories (one per leaf node).
// This is useful for manual testing and debugging.
func (p *Proxy) handleListHistories(c *fiber.Ctx) error {
	ctx := c.Context()

	// Get all leaf nodes (end points of conversations)
	leaves, err := p.storer.Leaves(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(llm.ErrorResponse{Error: "failed to get leaves"})
	}

	// Build history for each leaf
	histories := make([]HistoryResponse, 0, len(leaves))
	for _, leaf := range leaves {
		history, err := p.buildHistory(ctx, leaf.Hash)
		if err != nil {
			p.logger.Warn("failed to build history for leaf", zap.String("hash", leaf.Hash), zap.Error(err))
			continue
		}
		histories = append(histories, *history)
	}

	return c.JSON(map[string]any{
		"count":     len(histories),
		"histories": histories,
	})
}

// handleGetHistory returns the full conversation history leading up to a given node.
// The history is returned in chronological order (oldest first).
func (p *Proxy) handleGetHistory(c *fiber.Ctx) error {
	hash := c.Params("hash")
	if hash == "" {
		return c.Status(fiber.StatusBadRequest).JSON(llm.ErrorResponse{Error: "hash parameter required"})
	}

	history, err := p.buildHistory(c.Context(), hash)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(llm.ErrorResponse{Error: "node not found"})
	}

	return c.JSON(history)
}

// buildHistory constructs a HistoryResponse for the given node hash.
func (p *Proxy) buildHistory(ctx context.Context, hash string) (*HistoryResponse, error) {
	// Get the ancestry (returns newest first, i.e., from hash back to root)
	ancestry, err := p.storer.Ancestry(ctx, hash)
	if err != nil {
		return nil, err
	}

	// Convert to HistoryMessage and reverse to chronological order
	messages := make([]HistoryMessage, len(ancestry))
	for i, node := range ancestry {
		// Place in reverse order (oldest first)
		idx := len(ancestry) - 1 - i

		msg := HistoryMessage{
			Hash:       node.Hash,
			ParentHash: node.ParentHash,
		}

		// Extract role and content from the node's content map
		if content, ok := node.Content.(map[string]any); ok {
			if role, ok := content["role"].(string); ok {
				msg.Role = role
			}
			if contentStr, ok := content["content"].(string); ok {
				msg.Content = contentStr
			}
			if model, ok := content["model"].(string); ok {
				msg.Model = model
			}
			// Copy any additional metadata (excluding role, content, model, type)
			metadata := make(map[string]any)
			for k, v := range content {
				if k != "role" && k != "content" && k != "model" && k != "type" {
					metadata[k] = v
				}
			}
			if len(metadata) > 0 {
				msg.Metadata = metadata
			}
		}

		messages[idx] = msg
	}

	return &HistoryResponse{
		Messages: messages,
		HeadHash: hash,
		Depth:    len(messages),
	}, nil
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
