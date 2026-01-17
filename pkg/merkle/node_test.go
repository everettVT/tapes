package merkle_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/papercomputeco/tapes/pkg/merkle"
)

var _ = Describe("Node", func() {
	Describe("NewNode", func() {
		Context("when creating a root node (no parent)", func() {
			It("creates a node with the given content", func() {
				content := "hello world"
				node := merkle.NewNode(content, nil)

				Expect(node.Content).To(Equal(content))
			})

			It("sets ParentHash to nil for root nodes", func() {
				node := merkle.NewNode("test", nil)

				Expect(node.ParentHash).To(BeNil())
			})

			It("computes a non-empty hash", func() {
				node := merkle.NewNode("test", nil)

				Expect(node.Hash).NotTo(BeEmpty())
			})

			It("produces consistent hashes for the same content", func() {
				node1 := merkle.NewNode("same content", nil)
				node2 := merkle.NewNode("same content", nil)

				Expect(node1.Hash).To(Equal(node2.Hash))
			})

			It("produces different hashes for different content", func() {
				node1 := merkle.NewNode("content A", nil)
				node2 := merkle.NewNode("content B", nil)

				Expect(node1.Hash).NotTo(Equal(node2.Hash))
			})

			It("handles complex content types", func() {
				content := map[string]interface{}{
					"key":    "value",
					"number": 42,
				}
				node := merkle.NewNode(content, nil)

				Expect(node.Hash).NotTo(BeEmpty())
				Expect(node.Content).To(Equal(content))
			})
		})

		Context("when creating a child node (with parent)", func() {
			var parent *merkle.Node

			BeforeEach(func() {
				parent = merkle.NewNode("parent content", nil)
			})

			It("creates a child node with the given content", func() {
				child := merkle.NewNode("child content", parent)

				Expect(child.Content).To(Equal("child content"))
			})

			It("links the child to the parent via ParentHash", func() {
				child := merkle.NewNode("child content", parent)

				Expect(child.ParentHash).NotTo(BeNil())
				Expect(*child.ParentHash).To(Equal(parent.Hash))
			})

			It("computes a hash for the child node", func() {
				child := merkle.NewNode("child content", parent)

				Expect(child.Hash).NotTo(BeEmpty())
			})

			It("creates a chain of nodes", func() {
				child1 := merkle.NewNode("child 1", parent)
				child2 := merkle.NewNode("child 2", child1)
				child3 := merkle.NewNode("child 3", child2)

				Expect(parent.ParentHash).To(BeNil())
				Expect(*child1.ParentHash).To(Equal(parent.Hash))
				Expect(*child2.ParentHash).To(Equal(child1.Hash))
				Expect(*child3.ParentHash).To(Equal(child2.Hash))
			})

			It("produces different hashes for same content with different parents", func() {
				parent2 := merkle.NewNode("different parent", nil)
				child1 := merkle.NewNode("same content", parent)
				child2 := merkle.NewNode("same content", parent2)

				Expect(child1.Hash).NotTo(Equal(child2.Hash))
			})
		})
	})

	Describe("Hash computation", func() {
		It("produces a valid SHA-256 hex string (64 characters)", func() {
			node := merkle.NewNode("test", nil)

			Expect(node.Hash).To(HaveLen(64))
			Expect(node.Hash).To(MatchRegexp("^[a-f0-9]{64}$"))
		})
	})
})
