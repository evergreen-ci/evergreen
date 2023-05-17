package agent

import (
	"encoding/base64"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		files, err := getTraceFiles(tmpDir)
		assert.NoError(t, err)
		assert.Nil(t, files)
	})

	t.Run("EmptyTraceDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(path.Join(tmpDir, traceSuffix), os.ModePerm))
		files, err := getTraceFiles(tmpDir)
		assert.NoError(t, err)
		assert.Nil(t, files)
	})

	t.Run("PopulatedTraceDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(path.Join(tmpDir, traceSuffix), os.ModePerm))
		f, err := os.Create(path.Join(tmpDir, traceSuffix, "trace0.json"))
		require.NoError(t, err)
		require.NoError(t, f.Close())

		files, err := getTraceFiles(tmpDir)
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

		files, err := getTraceFiles(tmpDir)
		assert.NoError(t, err)
		require.Len(t, files, 1)
		assert.Equal(t, path.Join(tmpDir, traceSuffix, "trace0.json"), files[0])
	})

}
