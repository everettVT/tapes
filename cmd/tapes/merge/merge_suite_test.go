package mergecmder

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMergeCommander(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Merge Commander Suite")
}
