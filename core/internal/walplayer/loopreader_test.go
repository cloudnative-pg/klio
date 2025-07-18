package walplayer

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestLoopReader_Read_LoopsBuffer(t *testing.T) {
	// Test that LoopReader repeats the original data when reading more bytes
	// than the buffer contains. With data [1,2,3,4] and reading 10 bytes,
	// we expect: [1,2,3,4,1,2,3,4,1,2]
	data := []byte{1, 2, 3, 4}
	r := NewLoopReader(data)

	const readSize = 10
	buf := make([]byte, readSize)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != readSize {
		t.Errorf("expected to read %d bytes, got %d", readSize, n)
	}
	for i := range readSize {
		bufferIndex := i % len(data)
		expected := data[bufferIndex]
		if buf[i] != expected {
			t.Errorf("at %d: expected %d, got %d", i, expected, buf[i])
		}
	}
}

func TestLoopReader_Read_EmptyBuffer(t *testing.T) {
	r := NewLoopReader([]byte{})
	buf := make([]byte, 5)
	n, err := r.Read(buf)
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF, got %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 bytes read, got %d", n)
	}
}

func TestLoopReader_Read_SingleByte(t *testing.T) {
	data := []byte{42}
	r := NewLoopReader(data)
	buf := make([]byte, 3)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(buf, []byte{42, 42, 42}) {
		t.Errorf("expected all bytes to be 42, got %v", buf)
	}
	if n != 3 {
		t.Errorf("expected 3 bytes read, got %d", n)
	}
}
