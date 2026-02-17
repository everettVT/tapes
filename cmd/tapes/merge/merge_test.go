package mergecmder

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/papercomputeco/tapes/pkg/merkle"
	"github.com/papercomputeco/tapes/pkg/llm"
	"github.com/papercomputeco/tapes/pkg/storage/sqlite"
)

var _ = Describe("Merge Command", func() {
	var (
		ctx     context.Context
		tmpDir  string
		srcPath string
		dstPath string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tmpDir, err = os.MkdirTemp("", "tapes-merge-test-*")
		Expect(err).NotTo(HaveOccurred())
		srcPath = filepath.Join(tmpDir, "source.sqlite")
		dstPath = filepath.Join(tmpDir, "target.sqlite")
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

	It("merges nodes from source into target", func() {
		// Seed source with two nodes
		src, err := sqlite.NewDriver(ctx, srcPath)
		Expect(err).NotTo(HaveOccurred())
		nodeA := makeNode("user", "hello from source", nil)
		nodeB := makeNode("assistant", "hi back", nodeA)
		_, err = src.Put(ctx, nodeA)
		Expect(err).NotTo(HaveOccurred())
		_, err = src.Put(ctx, nodeB)
		Expect(err).NotTo(HaveOccurred())
		src.Close()

		// Seed target with one different node
		dst, err := sqlite.NewDriver(ctx, dstPath)
		Expect(err).NotTo(HaveOccurred())
		nodeC := makeNode("user", "hello from target", nil)
		_, err = dst.Put(ctx, nodeC)
		Expect(err).NotTo(HaveOccurred())
		dst.Close()

		// Run merge
		cmd := NewMergeCmd()
		cmd.SetArgs([]string{"--sqlite", dstPath, srcPath})
		err = cmd.ExecuteContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Verify target has all three nodes
		dst, err = sqlite.NewDriver(ctx, dstPath)
		Expect(err).NotTo(HaveOccurred())
		defer dst.Close()
		nodes, err := dst.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(nodes).To(HaveLen(3))
	})

	It("deduplicates when merging the same source twice", func() {
		// Seed source
		src, err := sqlite.NewDriver(ctx, srcPath)
		Expect(err).NotTo(HaveOccurred())
		nodeA := makeNode("user", "dedup test", nil)
		_, err = src.Put(ctx, nodeA)
		Expect(err).NotTo(HaveOccurred())
		src.Close()

		// Create empty target
		dst, err := sqlite.NewDriver(ctx, dstPath)
		Expect(err).NotTo(HaveOccurred())
		dst.Close()

		// Merge once
		cmd := NewMergeCmd()
		cmd.SetArgs([]string{"--sqlite", dstPath, srcPath})
		err = cmd.ExecuteContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Merge again
		cmd2 := NewMergeCmd()
		cmd2.SetArgs([]string{"--sqlite", dstPath, srcPath})
		err = cmd2.ExecuteContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Still only one node
		dst, err = sqlite.NewDriver(ctx, dstPath)
		Expect(err).NotTo(HaveOccurred())
		defer dst.Close()
		nodes, err := dst.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(nodes).To(HaveLen(1))
	})

	It("merges multiple sources", func() {
		src2Path := filepath.Join(tmpDir, "source2.sqlite")

		// Seed source 1
		src1, err := sqlite.NewDriver(ctx, srcPath)
		Expect(err).NotTo(HaveOccurred())
		nodeA := makeNode("user", "from source 1", nil)
		_, err = src1.Put(ctx, nodeA)
		Expect(err).NotTo(HaveOccurred())
		src1.Close()

		// Seed source 2
		src2, err := sqlite.NewDriver(ctx, src2Path)
		Expect(err).NotTo(HaveOccurred())
		nodeB := makeNode("user", "from source 2", nil)
		_, err = src2.Put(ctx, nodeB)
		Expect(err).NotTo(HaveOccurred())
		src2.Close()

		// Create empty target
		dst, err := sqlite.NewDriver(ctx, dstPath)
		Expect(err).NotTo(HaveOccurred())
		dst.Close()

		// Merge both sources
		cmd := NewMergeCmd()
		cmd.SetArgs([]string{"--sqlite", dstPath, srcPath, src2Path})
		err = cmd.ExecuteContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Target has both nodes
		dst, err = sqlite.NewDriver(ctx, dstPath)
		Expect(err).NotTo(HaveOccurred())
		defer dst.Close()
		nodes, err := dst.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(nodes).To(HaveLen(2))
	})
})
