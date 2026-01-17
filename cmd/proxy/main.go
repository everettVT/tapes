package main

import (
	"flag"

	"go.uber.org/zap"

	"github.com/papercomputeco/tapes/pkg/logger"
	"github.com/papercomputeco/tapes/proxy"
)

func main() {
	// Parse command line flags
	listenAddr := flag.String("listen", ":8080", "Address to listen on")
	upstreamURL := flag.String("upstream", "http://localhost:11434", "Upstream LLM provider URL (e.g., Ollama)")
	dbPath := flag.String("db", "", "Path to SQLite database (default: in-memory)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Set up logger
	logger := logger.NewLogger(*debug)
	defer logger.Sync()

	logger.Info("tapes LLM proxy starting",
		zap.String("listen", *listenAddr),
		zap.String("upstream", *upstreamURL),
		zap.Bool("debug", *debug),
	)

	// Create and run the proxy
	config := proxy.Config{
		ListenAddr:  *listenAddr,
		UpstreamURL: *upstreamURL,
		DBPath:      *dbPath,
	}

	p, err := proxy.New(config, logger)
	if err != nil {
		logger.Fatal("failed to create proxy", zap.Error(err))
	}
	defer p.Close()

	if err := p.Run(); err != nil {
		logger.Fatal("proxy server failed", zap.Error(err))
	}
}
