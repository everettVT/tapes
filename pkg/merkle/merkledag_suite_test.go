package merkle_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMerkle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Merkle DAG Suite")
}
