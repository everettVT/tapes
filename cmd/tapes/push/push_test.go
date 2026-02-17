package pushcmder

import (
	"context"
	"net"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	tapesapi "github.com/papercomputeco/tapes/api"
	"github.com/papercomputeco/tapes/pkg/llm"
	"github.com/papercomputeco/tapes/pkg/merkle"
	"github.com/papercomputeco/tapes/pkg/storage/inmemory"
	"github.com/papercomputeco/tapes/pkg/storage/sqlite"
)

var _ = Describe("Push Command", func() {
	var (
		ctx       context.Context
		tmpDir    string
		localPath string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tmpDir, err = os.MkdirTemp("", "tapes-push-test-*")
		Expect(err).NotTo(HaveOccurred())
		localPath = filepath.Join(tmpDir, "local.sqlite")
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	makeNode := func(role, text string, parent *merkle.Node) *merkle.Node {
		return merkle.NewNode(merkle.Bucket{
			Type:     "message",
			Role:     role,
			Content:  []llm.ContentBlock{{Type: "text", Text: text}},
			Model:    "test-model",
			Provider: "test",
		}, parent)
	}

	startServer := func() (string, *inmemory.Driver, func()) {
		serverDriver := inmemory.NewDriver()
		logger := zap.NewNop()

		srv, err := tapesapi.NewServer(tapesapi.Config{
			ListenAddr: ":0",
		}, serverDriver, serverDriver, logger)
		Expect(err).NotTo(HaveOccurred())

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		go func() {
			_ = srv.RunWithListener(listener)
		}()

		addr := "http://" + listener.Addr().String()
		cleanup := func() {
			srv.Shutdown()
		}
		return addr, serverDriver, cleanup
	}

	It("pushes local nodes to a remote server", func() {
		// Seed local DB
		local, err := sqlite.NewDriver(ctx, localPath)
		Expect(err).NotTo(HaveOccurred())
		nodeA := makeNode("user", "hello from push test", nil)
		nodeB := makeNode("assistant", "hi back from push test", nodeA)
		_, err = local.Put(ctx, nodeA)
		Expect(err).NotTo(HaveOccurred())
		_, err = local.Put(ctx, nodeB)
		Expect(err).NotTo(HaveOccurred())
		local.Close()

		// Start server
		addr, serverDriver, cleanup := startServer()
		defer cleanup()

		// Run push
		cmd := NewPushCmd()
		cmd.SetArgs([]string{"--sqlite", localPath, addr})
		err = cmd.ExecuteContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Verify server received nodes
		nodes, err := serverDriver.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(nodes).To(HaveLen(2))
	})

	It("deduplicates on double push", func() {
		// Seed local DB
		local, err := sqlite.NewDriver(ctx, localPath)
		Expect(err).NotTo(HaveOccurred())
		nodeA := makeNode("user", "dedup push test", nil)
		_, err = local.Put(ctx, nodeA)
		Expect(err).NotTo(HaveOccurred())
		local.Close()

		// Start server
		addr, serverDriver, cleanup := startServer()
		defer cleanup()

		// Push twice
		cmd1 := NewPushCmd()
		cmd1.SetArgs([]string{"--sqlite", localPath, addr})
		err = cmd1.ExecuteContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		cmd2 := NewPushCmd()
		cmd2.SetArgs([]string{"--sqlite", localPath, addr})
		err = cmd2.ExecuteContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Still only one node on server
		nodes, err := serverDriver.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(nodes).To(HaveLen(1))
	})
})
