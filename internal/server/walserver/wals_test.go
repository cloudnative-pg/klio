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
			assert.Equal(t, c.archivedName, getArchivedWALFileName(base, clusterName, c.walName))
		})
	}
}

func TestIsWALFileName(t *testing.T) {
	tests := []struct {
		walName   string
		isCorrect bool
	}{
		{
			walName:   "00000001000000760000007B",
			isCorrect: true,
		},
		{
			walName:   "00000002.history",
			isCorrect: true,
		},
		{
			walName:   "0000000100001234000055CD.007C9330.backup",
			isCorrect: true,
		},
		{
			walName:   "00000001000000760000007BA",
			isCorrect: false,
		},
		{
			walName:   "00000002.hostory",
			isCorrect: false,
		},
		{
			walName:   "0000001000000000000001F",
			isCorrect: false,
		},
	}

	for _, c := range tests {
		t.Run(c.walName, func(t *testing.T) {
			assert.Equal(t, c.isCorrect, isWALFileName(c.walName))
		})
	}
}
