package walserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetArchivedWALFileName(t *testing.T) {
	base := "/var/lib/klio/wals"
	clusterName := "cluster-example"

	tests := []struct {
		walName      string
		archivedName string
	}{
		{
			walName:      "00000001000000760000007B",
			archivedName: "/var/lib/klio/wals/cluster-example/0000000100000076/00000001000000760000007B",
		},
		{
			walName:      "00000001000000760000007B.partial",
			archivedName: "/var/lib/klio/wals/cluster-example/0000000100000076/00000001000000760000007B.partial",
		},
		{
			walName:      "00000002.history",
			archivedName: "/var/lib/klio/wals/cluster-example/00000002.history",
		},
		{
			walName:      "0000000100001234000055CD.007C9330.backup",
			archivedName: "/var/lib/klio/wals/cluster-example/0000000100001234000055CD.007C9330.backup",
		},
	}

	for _, c := range tests {
		t.Run(c.walName, func(t *testing.T) {
			assert.Equal(t, c.archivedName, getWALArchivePath(base, clusterName, c.walName))
		})
	}
}

func TestIsWALFileName(t *testing.T) {
	tests := []struct {
		walName string
		err     error
	}{
		{
			walName: "00000001000000760000007B",
			err:     nil,
		},
		{
			walName: "00000002.history",
			err:     nil,
		},
		{
			walName: "0000000100001234000055CD.007C9330.backup",
			err:     nil,
		},
		{
			walName: "00000001000000760000007BA",
			err:     NewIncorrectWALNameError("00000001000000760000007BA"),
		},
		{
			walName: "00000002.hostory",
			err:     NewIncorrectWALNameError("00000002.hostory"),
		},
		{
			walName: "0000001000000000000001F",
			err:     NewIncorrectWALNameError("0000001000000000000001F"),
		},
	}

	for _, c := range tests {
		t.Run(c.walName, func(t *testing.T) {
			assert.Equal(t, c.err, validateWalFileName(c.walName))
		})
	}
}
