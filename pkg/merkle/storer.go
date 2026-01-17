package merkle

import "context"

// Storer defines the interface for persisting and retrieving nodes in a Merkle DAG from a storage backend.
// The Storer is the primary interface for working with the Merkle DAG - it handles
// storage, retrieval, and traversal. De-duplication happens automatically via
// content-addressing: identical content with identical parents produces identical hashes
// and is stored appropriately by the implementers.
type Storer interface {
	// Put stores a node. If the node already exists (by hash), this is a no-op.
	// This provides automatic deduplication via content-addressing.
	Put(ctx context.Context, node *Node) error

	// Get retrieves a node by its hash. Returns ErrNotFound if the node doesn't exist.
	Get(ctx context.Context, hash string) (*Node, error)

	// Has checks if a node exists by its hash.
	Has(ctx context.Context, hash string) (bool, error)

	// GetByParent retrieves all nodes that have the given parent hash.
	// Pass nil to get root nodes (nodes with no parent).
	GetByParent(ctx context.Context, parentHash *string) ([]*Node, error)

	// List returns all nodes in the store.
	List(ctx context.Context) ([]*Node, error)

	// Roots returns all root nodes (nodes with no parent).
	Roots(ctx context.Context) ([]*Node, error)

	// Leaves returns all leaf nodes (nodes with no children).
	Leaves(ctx context.Context) ([]*Node, error)

	// Ancestry returns the path from a node back to its root (node first, root last).
	Ancestry(ctx context.Context, hash string) ([]*Node, error)

	// Descendants returns the path from root to node (root first, node last).
	Descendants(ctx context.Context, hash string) ([]*Node, error)

	// Depth returns the depth of a node (0 for roots).
	Depth(ctx context.Context, hash string) (int, error)

	// Close closes the store and releases any resources.
	Close() error
}

// ErrNotFound is returned when a node doesn't exist in the store.
type ErrNotFound struct {
	Hash string
}

func (e ErrNotFound) Error() string {
	if e.Hash == "" {
		return "node not found"
	}

	return "node not found: " + e.Hash
}
