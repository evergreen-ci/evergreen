package taskoutput

import (
	"context"
	"encoding/base64"
	"os"
	"path"
	"sync/atomic"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestUnmarshalTraces(t *testing.T) {
	resourceSpans, err := unmarshalTraces("./testdata/trace_data.json")
	assert.NoError(t, err)

	require.Len(t, resourceSpans, 7)
	require.Len(t, resourceSpans[0].ScopeSpans, 1)
	require.Len(t, resourceSpans[0].ScopeSpans[0].Spans, 262)
	assert.Equal(t, []byte{0xbb, 0xee, 0x44, 0x56, 0x2e, 0xd5, 0x30, 0x65, 0x32, 0x6b, 0x00, 0x8a, 0x38, 0xd8, 0x3a, 0x3c}, resourceSpans[0].ScopeSpans[0].Spans[0].TraceId)
	assert.Equal(t, []byte{0x18, 0xea, 0x05, 0x17, 0x51, 0x1d, 0x43, 0x86}, resourceSpans[0].ScopeSpans[0].Spans[0].SpanId)
	assert.Equal(t, []byte{}, resourceSpans[0].ScopeSpans[0].Spans[0].ParentSpanId)
	assert.Equal(t, uint64(1683818213402336000), resourceSpans[0].ScopeSpans[0].Spans[0].StartTimeUnixNano)
}

func TestFixBinaryID(t *testing.T) {
	t.Run("ValidID", func(t *testing.T) {
		base64DecodedID := []byte{0x6d, 0xb7, 0x9e, 0xe3, 0x8e, 0x7a, 0xd9, 0xe7, 0x79, 0xdf, 0x4e, 0xb9, 0xdf, 0x6e, 0x9b, 0xd3, 0x4f, 0x1a, 0xdf, 0xc7, 0x7c, 0xdd, 0xad, 0xdc}
		id, err := fixBinaryID(base64DecodedID)
		assert.NoError(t, err)
		assert.Equal(t, []byte{0xbb, 0xee, 0x44, 0x56, 0x2e, 0xd5, 0x30, 0x65, 0x32, 0x6b, 0x00, 0x8a, 0x38, 0xd8, 0x3a, 0x3c}, id)
	})

	t.Run("InvalidID", func(t *testing.T) {
		invalidHexID, err := base64.StdEncoding.DecodeString("invalidHexString")
		require.NoError(t, err)
		_, err = fixBinaryID(invalidHexID)
		assert.Error(t, err)
	})
}

func TestGetTraceFiles(t *testing.T) {
	t.Run("NoTraceDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		files, err := getTraceFiles(path.Join(tmpDir, traceSuffix))
		assert.NoError(t, err)
		assert.Nil(t, files)
	})

	t.Run("EmptyTraceDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(path.Join(tmpDir, traceSuffix), os.ModePerm))
		files, err := getTraceFiles(path.Join(tmpDir, traceSuffix))
		assert.NoError(t, err)
		assert.Nil(t, files)
	})

	t.Run("PopulatedTraceDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(path.Join(tmpDir, traceSuffix), os.ModePerm))
		f, err := os.Create(path.Join(tmpDir, traceSuffix, "trace0.json"))
		require.NoError(t, err)
		require.NoError(t, f.Close())

		files, err := getTraceFiles(path.Join(tmpDir, traceSuffix))
		assert.NoError(t, err)
		require.Len(t, files, 1)
		assert.Equal(t, path.Join(tmpDir, traceSuffix, "trace0.json"), files[0])
	})

	t.Run("TraceDirContainsDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(path.Join(tmpDir, traceSuffix, "nested_directory"), os.ModePerm))
		f, err := os.Create(path.Join(tmpDir, traceSuffix, "trace0.json"))
		require.NoError(t, err)
		require.NoError(t, f.Close())

		f, err = os.Create(path.Join(tmpDir, traceSuffix, "nested_directory", "trace1.json"))
		require.NoError(t, err)
		require.NoError(t, f.Close())

		files, err := getTraceFiles(path.Join(tmpDir, traceSuffix))
		assert.NoError(t, err)
		require.Len(t, files, 1)
		assert.Equal(t, path.Join(tmpDir, traceSuffix, "trace0.json"), files[0])
	})

}

func TestBatchSpans(t *testing.T) {
	for _, testCase := range []struct {
		name               string
		spansLen           int
		batchSize          int
		expectedBatchCount int
	}{
		{
			name:               "EvenSize",
			spansLen:           100,
			batchSize:          10,
			expectedBatchCount: 10,
		},
		{
			name:               "OddSize",
			spansLen:           10,
			batchSize:          3,
			expectedBatchCount: 4,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var spans []*tracepb.ResourceSpans
			for i := 0; i < testCase.spansLen; i++ {
				spans = append(spans, &tracepb.ResourceSpans{})
			}

			spanBatches := batchSpans(spans, testCase.batchSize)
			assert.Len(t, spanBatches, testCase.expectedBatchCount)
			for _, batch := range spanBatches[:len(spanBatches)-1] {
				assert.Len(t, batch, testCase.spansLen/testCase.batchSize)
			}
		})
	}
}

// mockTraceClient implements otlptrace.Client for testing.
type mockTraceClient struct {
	uploadCount atomic.Int64
}

func (m *mockTraceClient) Start(ctx context.Context) error { return nil }
func (m *mockTraceClient) Stop(ctx context.Context) error  { return nil }
func (m *mockTraceClient) UploadTraces(ctx context.Context, protoSpans []*tracepb.ResourceSpans) error {
	m.uploadCount.Add(1)
	return nil
}

func TestOtelTraceDirectoryHandlerRun(t *testing.T) {
	t.Run("ProcessesMultipleFiles", func(t *testing.T) {
		// Create temp directory with multiple trace files.
		tmpDir := t.TempDir()

		// Copy testdata trace file multiple times.
		srcData, err := os.ReadFile("./testdata/trace_data.json")
		require.NoError(t, err)

		numFiles := 5
		for i := 0; i < numFiles; i++ {
			err := os.WriteFile(path.Join(tmpDir, "trace"+string(rune('0'+i))+".json"), srcData, 0644)
			require.NoError(t, err)
		}

		mockComm := client.NewMock("http://localhost")
		logger, err := mockComm.GetLoggerProducer(t.Context(), &task.Task{Id: "test"}, nil)
		require.NoError(t, err)

		mockClient := &mockTraceClient{}
		handler := &otelTraceDirectoryHandler{
			dir:         tmpDir,
			logger:      logger,
			traceClient: mockClient,
		}

		err = handler.run(t.Context())
		assert.NoError(t, err)

		// Verify uploads happened (5 files were created).
		assert.Equal(t, mockClient.uploadCount.Load(), int64(5))

		// Verify all files were removed.
		files, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		assert.Empty(t, files, "all trace files should be removed after processing")
	})

	t.Run("HandlesEmptyDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()

		mockComm := client.NewMock("http://localhost")
		logger, err := mockComm.GetLoggerProducer(t.Context(), &task.Task{Id: "test"}, nil)
		require.NoError(t, err)

		mockClient := &mockTraceClient{}
		handler := &otelTraceDirectoryHandler{
			dir:         tmpDir,
			logger:      logger,
			traceClient: mockClient,
		}

		err = handler.run(t.Context())
		require.NoError(t, err)
		assert.Equal(t, int64(0), mockClient.uploadCount.Load())
	})
}
