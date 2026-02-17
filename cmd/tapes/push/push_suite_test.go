package pushcmder

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPushCommander(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Push Commander Suite")
}
