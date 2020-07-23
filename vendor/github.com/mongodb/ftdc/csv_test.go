package ftdc

import (
	"bytes"
	"context"
	"encoding/csv"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/ftdc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCSVIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tmp, err := ioutil.TempDir("", "ftdc-csv-")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.RemoveAll(tmp)) }()

	t.Run("Write", func(t *testing.T) {
		iter := ReadChunks(ctx, bytes.NewBuffer(newChunk(10)))
		out := &bytes.Buffer{}
		err := WriteCSV(ctx, iter, out)
		require.NoError(t, err)

		lines := strings.Split(out.String(), "\n")
		assert.Len(t, lines, 12)
	})
	t.Run("ResuseIterPass", func(t *testing.T) {
		iter := ReadChunks(ctx, bytes.NewBuffer(newChunk(10)))
		err := DumpCSV(ctx, iter, filepath.Join(tmp, "dump"))
		require.NoError(t, err)
		err = DumpCSV(ctx, iter, filepath.Join(tmp, "dump"))
		require.NoError(t, err)
	})
	t.Run("Dump", func(t *testing.T) {
		iter := ReadChunks(ctx, bytes.NewBuffer(newChunk(10)))
		err := DumpCSV(ctx, iter, filepath.Join(tmp, "dump"))
		require.NoError(t, err)
	})
	t.Run("DumpMixed", func(t *testing.T) {
		iter := ReadChunks(ctx, bytes.NewBuffer(newMixedChunk(10)))
		err := DumpCSV(ctx, iter, filepath.Join(tmp, "dump"))
		require.NoError(t, err)
	})
	t.Run("WriteWithSchemaChange", func(t *testing.T) {
		iter := ReadChunks(ctx, bytes.NewBuffer(newMixedChunk(10)))
		out := &bytes.Buffer{}
		err := WriteCSV(ctx, iter, out)

		require.Error(t, err)
	})
}

func TestReadCSVIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, test := range []struct {
		Name   string
		Iter   *ChunkIterator
		Rows   int
		Fields int
	}{
		{
			Name:   "SimpleFlat",
			Iter:   produceMockChunkIter(ctx, 1000, func() *birch.Document { return testutil.RandFlatDocument(15) }),
			Rows:   1000,
			Fields: 15,
		},
		{
			Name:   "LargerFlat",
			Iter:   produceMockChunkIter(ctx, 1000, func() *birch.Document { return testutil.RandFlatDocument(50) }),
			Rows:   1000,
			Fields: 50,
		},
		{
			Name:   "Complex",
			Iter:   produceMockChunkIter(ctx, 1000, func() *birch.Document { return testutil.RandComplexDocument(20, 3) }),
			Rows:   1000,
			Fields: 100,
		},
		{
			Name:   "LargComplex",
			Iter:   produceMockChunkIter(ctx, 1000, func() *birch.Document { return testutil.RandComplexDocument(100, 10) }),
			Rows:   1000,
			Fields: 190,
		},
	} {
		t.Run(test.Name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := WriteCSV(ctx, test.Iter, buf)
			require.NoError(t, err)

			out := &bytes.Buffer{}
			err = ConvertFromCSV(ctx, test.Rows, buf, out)
			require.NoError(t, err)

			iter := ReadMetrics(ctx, out)
			count := 0
			for iter.Next() {
				count++
				doc := iter.Document()
				assert.Equal(t, test.Fields, doc.Len())
			}
			assert.Equal(t, test.Rows, count)
		})
	}
	t.Run("SchemaChangeGrow", func(t *testing.T) {
		buf := &bytes.Buffer{}
		csvw := csv.NewWriter(buf)
		require.NoError(t, csvw.Write([]string{"a", "b", "c", "d"}))
		for j := 0; j < 2; j++ {
			for i := 0; i < 10; i++ {
				require.NoError(t, csvw.Write([]string{"1", "2", "3", "4"}))
			}
			require.NoError(t, csvw.Write([]string{"1", "2", "3", "4", "5"}))
		}
		csvw.Flush()

		assert.Error(t, ConvertFromCSV(ctx, 1000, buf, &bytes.Buffer{}))
	})
	t.Run("SchemaChangeShrink", func(t *testing.T) {
		buf := &bytes.Buffer{}
		csvw := csv.NewWriter(buf)
		require.NoError(t, csvw.Write([]string{"a", "b", "c", "d"}))
		for j := 0; j < 2; j++ {
			for i := 0; i < 10; i++ {
				require.NoError(t, csvw.Write([]string{"1", "2", "3", "4"}))
			}
			require.NoError(t, csvw.Write([]string{"1", "2"}))
		}
		csvw.Flush()

		assert.Error(t, ConvertFromCSV(ctx, 1000, buf, &bytes.Buffer{}))
	})
}
