package tier1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTier1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tier1 Suite")
}
