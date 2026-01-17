// Package merkle is an implementation of a Merkel DAG
package merkle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Node represents a single content-addressed node in a Merkle DAG
type Node struct {
	// Hash is the content-addressed identifier (SHA-256, hex-encoded)
	Hash string `json:"hash"`

	// ParentHash links to the previous node hash.
	// This will be nil for root nodes.
	ParentHash *string `json:"parent_hash"`

	// Content is the hashable content for the node
	Content any `json:"content"`
}

// NewNode creates a new node with the computed hash for the provided content
func NewNode(content any, parent *Node) *Node {
	n := &Node{
		Content: content,
	}

	if parent != nil {
		n.ParentHash = &parent.Hash
	}

	n.Hash = n.computeHash()
	return n
}

// ComputeHash calculates the content-addressed hash for a node
func (n *Node) computeHash() string {
	i := &input{
		Content: n.Content,
	}

	if n.ParentHash != nil {
		i.Parent = *n.ParentHash
	}

	// Canonical JSON encoding for deterministic hashing
	data, err := json.Marshal(i)
	if err != nil {
		panic("failed to marshal hash input: " + err.Error())
	}

	h := sha256.Sum256(data)
	computed := hex.EncodeToString(h[:])
	return computed
}
