package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/papercomputeco/tapes/pkg/merkle"
)

// testProxy creates a Proxy with an in-memory storer for testing.
func testProxy(t *testing.T) *Proxy {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	return &Proxy{
		config: Config{
			ListenAddr:  ":0",
			UpstreamURL: "http://localhost:11434", // not used in these tests
		},
		storer: merkle.NewMemoryStorer(),
		logger: logger,
	}
}

// testApp creates a Fiber app with the proxy routes for testing.
func testApp(t *testing.T, p *Proxy) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(map[string]string{"status": "ok"})
	})
	app.Get("/dag/stats", p.handleDAGStats)
	app.Get("/dag/node/:hash", p.handleGetNode)
	app.Get("/dag/history", p.handleListHistories)
	app.Get("/dag/history/:hash", p.handleGetHistory)
	return app
}

func TestHealthEndpoint(t *testing.T) {
	p := testProxy(t)
	app := testApp(t, p)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]string
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "ok", result["status"])
}

func TestDAGStatsEmpty(t *testing.T) {
	p := testProxy(t)
	app := testApp(t, p)

	req := httptest.NewRequest("GET", "/dag/stats", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var stats map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &stats))

	assert.Equal(t, float64(0), stats["total_nodes"])
	assert.Equal(t, float64(0), stats["root_count"])
	assert.Equal(t, float64(0), stats["leaf_count"])
}

func TestDAGStatsWithNodes(t *testing.T) {
	p := testProxy(t)
	app := testApp(t, p)
	ctx := context.Background()

	// Add some nodes
	node1 := merkle.NewNode(map[string]string{"role": "user", "content": "Hello"}, nil)
	node2 := merkle.NewNode(map[string]string{"role": "assistant", "content": "Hi there!"}, node1)
	require.NoError(t, p.storer.Put(ctx, node1))
	require.NoError(t, p.storer.Put(ctx, node2))

	req := httptest.NewRequest("GET", "/dag/stats", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var stats map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &stats))

	assert.Equal(t, float64(2), stats["total_nodes"])
	assert.Equal(t, float64(1), stats["root_count"])
	assert.Equal(t, float64(1), stats["leaf_count"])
}

func TestGetNode(t *testing.T) {
	p := testProxy(t)
	app := testApp(t, p)
	ctx := context.Background()

	// Add a node
	node := merkle.NewNode(map[string]string{"role": "user", "content": "Hello"}, nil)
	require.NoError(t, p.storer.Put(ctx, node))

	req := httptest.NewRequest("GET", "/dag/node/"+node.Hash, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result merkle.Node
	require.NoError(t, json.Unmarshal(body, &result))

	assert.Equal(t, node.Hash, result.Hash)
	assert.Nil(t, result.ParentHash)
}

func TestGetNodeNotFound(t *testing.T) {
	p := testProxy(t)
	app := testApp(t, p)

	req := httptest.NewRequest("GET", "/dag/node/nonexistent", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetHistory(t *testing.T) {
	p := testProxy(t)
	app := testApp(t, p)
	ctx := context.Background()

	// Build a conversation chain
	node1 := merkle.NewNode(map[string]interface{}{
		"type":    "message",
		"role":    "user",
		"content": "Hello",
		"model":   "test-model",
	}, nil)
	node2 := merkle.NewNode(map[string]interface{}{
		"type":    "message",
		"role":    "assistant",
		"content": "Hi there!",
		"model":   "test-model",
	}, node1)
	node3 := merkle.NewNode(map[string]interface{}{
		"type":    "message",
		"role":    "user",
		"content": "How are you?",
		"model":   "test-model",
	}, node2)

	require.NoError(t, p.storer.Put(ctx, node1))
	require.NoError(t, p.storer.Put(ctx, node2))
	require.NoError(t, p.storer.Put(ctx, node3))

	req := httptest.NewRequest("GET", "/dag/history/"+node3.Hash, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var history HistoryResponse
	require.NoError(t, json.Unmarshal(body, &history))

	assert.Equal(t, node3.Hash, history.HeadHash)
	assert.Equal(t, 3, history.Depth)
	require.Len(t, history.Messages, 3)

	// Messages should be in chronological order (oldest first)
	assert.Equal(t, "user", history.Messages[0].Role)
	assert.Equal(t, "Hello", history.Messages[0].Content)
	assert.Nil(t, history.Messages[0].ParentHash) // root node

	assert.Equal(t, "assistant", history.Messages[1].Role)
	assert.Equal(t, "Hi there!", history.Messages[1].Content)
	assert.NotNil(t, history.Messages[1].ParentHash)

	assert.Equal(t, "user", history.Messages[2].Role)
	assert.Equal(t, "How are you?", history.Messages[2].Content)
	assert.NotNil(t, history.Messages[2].ParentHash)
}

func TestGetHistoryNotFound(t *testing.T) {
	p := testProxy(t)
	app := testApp(t, p)

	req := httptest.NewRequest("GET", "/dag/history/nonexistent", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestListHistories(t *testing.T) {
	p := testProxy(t)
	app := testApp(t, p)
	ctx := context.Background()

	// Build two separate conversations
	// Conversation 1
	conv1Msg1 := merkle.NewNode(map[string]interface{}{
		"type": "message", "role": "user", "content": "Hello", "model": "test",
	}, nil)
	conv1Msg2 := merkle.NewNode(map[string]interface{}{
		"type": "message", "role": "assistant", "content": "Hi!", "model": "test",
	}, conv1Msg1)

	// Conversation 2
	conv2Msg1 := merkle.NewNode(map[string]interface{}{
		"type": "message", "role": "user", "content": "What is Go?", "model": "test",
	}, nil)
	conv2Msg2 := merkle.NewNode(map[string]interface{}{
		"type": "message", "role": "assistant", "content": "A programming language.", "model": "test",
	}, conv2Msg1)

	require.NoError(t, p.storer.Put(ctx, conv1Msg1))
	require.NoError(t, p.storer.Put(ctx, conv1Msg2))
	require.NoError(t, p.storer.Put(ctx, conv2Msg1))
	require.NoError(t, p.storer.Put(ctx, conv2Msg2))

	req := httptest.NewRequest("GET", "/dag/history", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Count     int               `json:"count"`
		Histories []HistoryResponse `json:"histories"`
	}
	require.NoError(t, json.Unmarshal(body, &result))

	assert.Equal(t, 2, result.Count)
	require.Len(t, result.Histories, 2)

	// Each history should have 2 messages
	for _, h := range result.Histories {
		assert.Equal(t, 2, h.Depth)
		assert.Len(t, h.Messages, 2)
	}
}

func TestListHistoriesEmpty(t *testing.T) {
	p := testProxy(t)
	app := testApp(t, p)

	req := httptest.NewRequest("GET", "/dag/history", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Count     int               `json:"count"`
		Histories []HistoryResponse `json:"histories"`
	}
	require.NoError(t, json.Unmarshal(body, &result))

	assert.Equal(t, 0, result.Count)
	assert.Len(t, result.Histories, 0)
}

func TestContentAddressableDeduplication(t *testing.T) {
	p := testProxy(t)
	ctx := context.Background()

	// Create the same node twice - should only store once
	content := map[string]string{"role": "user", "content": "Hello"}
	node1 := merkle.NewNode(content, nil)
	node2 := merkle.NewNode(content, nil)

	// Same content = same hash
	assert.Equal(t, node1.Hash, node2.Hash)

	// Store both
	require.NoError(t, p.storer.Put(ctx, node1))
	require.NoError(t, p.storer.Put(ctx, node2))

	// Should only have one node
	nodes, err := p.storer.List(ctx)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
}

func TestBranchingConversations(t *testing.T) {
	p := testProxy(t)
	ctx := context.Background()

	// Common prefix
	userMsg := merkle.NewNode(map[string]string{
		"role":    "user",
		"content": "What is 2+2?",
	}, nil)
	require.NoError(t, p.storer.Put(ctx, userMsg))

	// Two different responses (simulating different LLM outputs)
	response1 := merkle.NewNode(map[string]string{
		"role":    "assistant",
		"content": "2+2 equals 4.",
	}, userMsg)
	response2 := merkle.NewNode(map[string]string{
		"role":    "assistant",
		"content": "The answer is 4!",
	}, userMsg)

	require.NoError(t, p.storer.Put(ctx, response1))
	require.NoError(t, p.storer.Put(ctx, response2))

	// Different content = different hashes
	assert.NotEqual(t, response1.Hash, response2.Hash)

	// But same parent
	assert.Equal(t, *response1.ParentHash, *response2.ParentHash)
	assert.Equal(t, userMsg.Hash, *response1.ParentHash)

	// Should have 3 nodes total (1 user + 2 branches)
	nodes, err := p.storer.List(ctx)
	require.NoError(t, err)
	assert.Len(t, nodes, 3)

	// 1 root, 2 leaves
	roots, err := p.storer.Roots(ctx)
	require.NoError(t, err)
	assert.Len(t, roots, 1)

	leaves, err := p.storer.Leaves(ctx)
	require.NoError(t, err)
	assert.Len(t, leaves, 2)
}
