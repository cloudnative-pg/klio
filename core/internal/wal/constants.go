package wal

// MaxBlockSizeBytes is the maximum WAL block payload size (without gRPC framing) accepted by the server.
// Client-side commands sending WAL data must keep each message <= this size.
const MaxBlockSizeBytes = 8 * 1024 * 1024 // 8 MiB

// GRPCOverheadBytes is the extra allowance we add server-side to account for gRPC framing
// (compressed proto headers, envelopes, etc.). Empirically sized at 512 KiB to safely
// accommodate framing for an 8 MiB payload.
const GRPCOverheadBytes = 512 * 1024 // 512 KiB

// MaxGRPCMessageSizeBytes is the maximum total message size (payload + gRPC framing) we accept/send.
const MaxGRPCMessageSizeBytes = MaxBlockSizeBytes + GRPCOverheadBytes

// GRPCInitialWindowSizeBytes is the HTTP/2 per-stream flow-control window used by both
// the WAL gRPC client and server. Sized at 2× MaxBlockSizeBytes so that streaming a
// WAL block does not require multiple WINDOW_UPDATE round trips.
const GRPCInitialWindowSizeBytes = 16 * 1024 * 1024 // 16 MiB

// GRPCInitialConnWindowSizeBytes is the HTTP/2 per-connection flow-control window. It must
// be at least as large as GRPCInitialWindowSizeBytes; we oversize it to leave room for
// multiple concurrent streams on the same connection.
const GRPCInitialConnWindowSizeBytes = 64 * 1024 * 1024 // 64 MiB

// GRPCSocketBufferSizeBytes is the TCP socket read/write buffer size hint for the gRPC
// transport. Larger values help sustain high throughput on links with non-trivial RTT.
const GRPCSocketBufferSizeBytes = 1 * 1024 * 1024 // 1 MiB
