package tier1

import (
	"github.com/kopia/kopia/repo/manifest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("getLatestWALFileNameFromManifests", func() {
	It("returns the latest WAL file name when manifests are provided", func() {
		manifests := []*manifest.EntryMetadata{
			{Labels: map[string]string{"path": "/wal/000000010000000000000001"}},
			{Labels: map[string]string{"path": "/wal/000000010000000000000002"}},
		}
		walSegmentSize := uint64(16 * 1024 * 1024)

		latestWAL, err := getLatestWALFileNameFromManifests(manifests, walSegmentSize)
		Expect(err).NotTo(HaveOccurred())
		Expect(latestWAL).To(Equal("000000010000000000000002"))
	})

	It("returns an error when no manifests are provided", func() {
		manifests := []*manifest.EntryMetadata{}
		walSegmentSize := uint64(16 * 1024 * 1024)

		latestWAL, err := getLatestWALFileNameFromManifests(manifests, walSegmentSize)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("no manifests passed"))
		Expect(latestWAL).To(BeEmpty())
	})

	It("returns an error when LSN cannot be parsed from WAL name", func() {
		manifests := []*manifest.EntryMetadata{
			{Labels: map[string]string{"path": "/wal/invalid_wal_name"}},
		}
		walSegmentSize := uint64(16 * 1024 * 1024)

		latestWAL, err := getLatestWALFileNameFromManifests(manifests, walSegmentSize)
		Expect(err).To(HaveOccurred())
		Expect(latestWAL).To(BeEmpty())
	})
})
