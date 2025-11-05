package walserver

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
)

// TestNewWithReadOnlyTrue verifies that New correctly sets isReadOnly field.
func TestNewWithReadOnlyTrue(t *testing.T) {
	opts := Options{
		Connection: nil,
		ReadOnly:   true,
	}

	impl := New(opts)

	if !impl.isReadOnly {
		t.Fatal("expected isReadOnly to be true")
	}

	if impl.metrics == nil {
		t.Fatal("expected metrics to be initialized")
	}
}

// TestNewWithReadOnlyFalse verifies that New correctly sets isReadOnly field to false.
func TestNewWithReadOnlyFalse(t *testing.T) {
	opts := Options{
		Connection: nil,
		ReadOnly:   false,
	}

	impl := New(opts)

	if impl.isReadOnly {
		t.Fatal("expected isReadOnly to be false")
	}

	if impl.metrics == nil {
		t.Fatal("expected metrics to be initialized")
	}
}

// TestSetFirstRequiredWALReadOnlyMode verifies that SetFirstRequiredWAL returns an error
// when the server is in read-only mode.
func TestSetFirstRequiredWALReadOnlyMode(t *testing.T) {
	opts := Options{
		Connection: nil,
		ReadOnly:   true,
	}

	impl := New(opts)

	req := &grpc.SetFirstRequiredWALRequest{
		ClusterName:      "test-cluster",
		FirstRequiredWal: "000000010000000000000001",
	}

	result, err := impl.SetFirstRequiredWAL(context.Background(), req)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if result != nil {
		t.Fatal("expected nil result")
	}

	// Verify that the error is a FailedPrecondition status code
	s, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}

	if s.Code() != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", s.Code())
	}

	if s.Message() != errReadOnly.Error() {
		t.Fatalf("expected message %q, got %q", errReadOnly.Error(), s.Message())
	}
}

// TestResetWALStreamReadOnlyMode verifies that ResetWALStream returns an error
// when the server is in read-only mode.
func TestResetWALStreamReadOnlyMode(t *testing.T) {
	opts := Options{
		Connection: nil,
		ReadOnly:   true,
	}

	impl := New(opts)

	req := &grpc.ResetWALStreamRequest{
		ClusterName:    "test-cluster",
		SystemId:       "12345",
		CurrentWalName: "000000010000000000000001",
	}

	result, err := impl.ResetWALStream(context.Background(), req)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if result != nil {
		t.Fatal("expected nil result")
	}

	// Verify that the error is a FailedPrecondition status code
	s, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}

	if s.Code() != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", s.Code())
	}

	if s.Message() != errReadOnly.Error() {
		t.Fatalf("expected message %q, got %q", errReadOnly.Error(), s.Message())
	}
}

// TestRequestWALStartReadOnlyMode verifies that RequestWALStart returns an error
// when the server is in read-only mode.
func TestRequestWALStartReadOnlyMode(t *testing.T) {
	opts := Options{
		Connection: nil,
		ReadOnly:   true,
	}

	impl := New(opts)

	req := &grpc.RequestWALStartRequest{
		ClusterName:    "test-cluster",
		SystemId:       "12345",
		CurrentWalName: "000000010000000000000001",
	}

	result, err := impl.RequestWALStart(context.Background(), req)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if result != nil {
		t.Fatal("expected nil result")
	}

	// Verify that the error is a FailedPrecondition status code
	s, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}

	if s.Code() != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", s.Code())
	}

	if s.Message() != errReadOnly.Error() {
		t.Fatalf("expected message %q, got %q", errReadOnly.Error(), s.Message())
	}
}

// mockPutServer is a minimal mock implementation of grpc.WAL_PutServer for testing.
type mockPutServer struct {
	grpc.WAL_PutServer
}

func (m *mockPutServer) Context() context.Context {
	return context.Background()
}

func (m *mockPutServer) Recv() (*grpc.PutRequest, error) {
	// This should never be called in the read-only test
	// since the check happens before any Recv()
	return nil, nil
}

// TestPutReadOnlyMode verifies that Put returns an error
// when the server is in read-only mode.
func TestPutReadOnlyMode(t *testing.T) {
	opts := Options{
		Connection: nil,
		ReadOnly:   true,
	}

	impl := New(opts)

	mockServer := &mockPutServer{}

	err := impl.Put(mockServer)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify that the error is a FailedPrecondition status code
	s, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}

	if s.Code() != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", s.Code())
	}

	if s.Message() != errReadOnly.Error() {
		t.Fatalf("expected message %q, got %q", errReadOnly.Error(), s.Message())
	}
}
