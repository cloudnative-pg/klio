/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

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
