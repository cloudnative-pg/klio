package infrastructure

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("WAL segment size parser", func() {
	DescribeTable(
		"parser",
		func(size string, expectedResult uint64, shouldFail bool) {
			result, err := parseWALSegmentSize(size)
			if shouldFail {
				Expect(err).To(HaveOccurred())
				Expect(expectedResult).To(BeZero())
				Expect(result).To(BeZero())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(expectedResult))
			}
		},
		Entry("correct size", "1024KB", uint64(1024*1024), false),
		Entry("correct size", "16MB", uint64(16*1024*1024), false),
		Entry("correct size", "1MB", uint64(1*1024*1024), false),
		Entry("no suffix", "1", uint64(1), false),
		Entry("correct size, unknown suffix", "12AP", uint64(0), true),
		Entry("wrong size, known suffix", "1A2GB", uint64(0), true),
		Entry("wrong size, known suffix", "1A2GB", uint64(0), true),
	)
})
