package queue

import (
	"encoding/json"
	"testing"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockMsg and MockBatch implementation remains the same.
type MockMsg struct {
	// Embedding the interface satisfies all methods of jetstream.Msg
	jetstream.Msg

	data []byte
}

// Override ONLY the method your code actually uses.
func (m *MockMsg) Data() []byte {
	return m.data
}

type MockBatch struct {
	messages []jetstream.Msg
	err      error
}

func (m *MockBatch) Messages() <-chan jetstream.Msg {
	ch := make(chan jetstream.Msg, len(m.messages))
	for _, msg := range m.messages {
		ch <- msg
	}
	close(ch)

	return ch
}

func (m *MockBatch) Error() error { return m.err }

func TestExtractOldestWALFromMessages(t *testing.T) {
	cluster := "my-cluster"

	tests := []struct {
		name           string
		clusterName    string
		setupMessages  func(t *testing.T) []jetstream.Msg
		batchError     error
		expectedResult string
	}{
		{
			name:        "Finds the lexicographically oldest WAL",
			clusterName: cluster,
			setupMessages: func(t *testing.T) []jetstream.Msg {
				t.Helper()
				return []jetstream.Msg{
					&MockMsg{data: marshalTask(t, cluster, "wal-2023-02")},
					&MockMsg{data: marshalTask(t, cluster, "wal-2023-01")},
					&MockMsg{data: marshalTask(t, cluster, "wal-2023-03")},
				}
			},
			expectedResult: "wal-2023-01",
		},
		{
			name:        "Skips invalid JSON and continues",
			clusterName: cluster,
			setupMessages: func(t *testing.T) []jetstream.Msg {
				t.Helper()
				return []jetstream.Msg{
					&MockMsg{data: []byte(`{!!invalid!!}`)},
					&MockMsg{data: marshalTask(t, cluster, "wal-999")},
				}
			},
			expectedResult: "wal-999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := tt.setupMessages(t)
			batch := &MockBatch{messages: messages, err: tt.batchError}
			logger := log.FromContext(t.Context())

			result := extractOldestWALFromMessages(logger, batch, tt.clusterName)

			assert.Equal(t, tt.expectedResult, result, "The extracted WAL name is incorrect")

			require.NoError(t, batch.Error())
		})
	}
}

func marshalTask(t *testing.T, cluster, wal string) []byte {
	t.Helper()

	data, err := json.Marshal(WALTask{
		ClusterName: cluster,
		WALName:     wal,
	})
	// If marshaling fails, the test setup is broken; stop immediately.
	require.NoError(t, err, "Failed to marshal test data")

	return data
}
