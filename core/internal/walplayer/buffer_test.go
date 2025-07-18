package walplayer

import (
	"testing"
)

func TestBufferEmbedded(t *testing.T) {
	if len(buffer) == 0 {
		t.Fatal("embedded buffer is empty - ensure 'buffer' file exists in the package directory")
	}

	// TODO: add a better cutoff
	if len(buffer) < 100 {
		t.Errorf("buffer seems too small (%d bytes) - might not be compiling correctly", len(buffer))
	}
}
