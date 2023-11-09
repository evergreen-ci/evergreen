package log

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/evergreen-ci/pail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogIteratorReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: t.TempDir()})
	require.NoError(t, err)

	chunks, lines, parser, err := generateTestLog(ctx, bucket, 100, 10)
	require.NoError(t, err)

	tz, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	for _, test := range []struct {
		name          string
		it            LogIterator
		opts          LogIteratorReaderOptions
		expectedLines []LogLine
		formatLine    func(LogLine) string
	}{
		{
			name:          "LineDataOnly",
			it:            newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket, chunks: chunks, parser: parser}),
			formatLine:    func(line LogLine) string { return line.Data + "\n" },
			expectedLines: lines,
		},
		{
			name:          "PrintTimeDefaultTimeZone",
			it:            newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket, chunks: chunks, parser: parser}),
			opts:          LogIteratorReaderOptions{PrintTime: true},
			expectedLines: lines,
			formatLine: func(line LogLine) string {
				return fmt.Sprintf("[%s] %s\n", time.Unix(0, line.Timestamp).UTC().Format("2006/01/02 15:04:05.000"), line.Data)
			},
		},
		{
			name: "PrintTimeSpecifiedTimeZone",
			it:   newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket, chunks: chunks, parser: parser}),
			opts: LogIteratorReaderOptions{
				PrintTime: true,
				TimeZone:  tz,
			},
			expectedLines: lines,
			formatLine: func(line LogLine) string {
				return fmt.Sprintf("[%s] %s\n", time.Unix(0, line.Timestamp).In(tz).Format("2006/01/02 15:04:05.000"), line.Data)
			},
		},
		{
			name:          "PrintPriority",
			it:            newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket, chunks: chunks, parser: parser}),
			opts:          LogIteratorReaderOptions{PrintPriority: true},
			expectedLines: lines,
			formatLine:    func(line LogLine) string { return fmt.Sprintf("[P:%3d] %s\n", line.Priority, line.Data) },
		},
		{
			name: "PrintTimeAndPriority",
			it:   newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket, chunks: chunks, parser: parser}),
			opts: LogIteratorReaderOptions{
				PrintTime:     true,
				PrintPriority: true,
			},
			expectedLines: lines,
			formatLine: func(line LogLine) string {
				return fmt.Sprintf("[P:%3d] [%s] %s\n", line.Priority, time.Unix(0, line.Timestamp).UTC().Format("2006/01/02 15:04:05.000"), line.Data)
			},
		},
		{
			name: "SoftSizeLimit",
			it: newMergingIterator(
				newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket, chunks: chunks, parser: parser}),
				newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket, chunks: chunks, parser: parser}),
				newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket, chunks: chunks, parser: parser}),
			),
			opts: LogIteratorReaderOptions{SoftSizeLimit: 5000},
			expectedLines: func() []LogLine {
				// With a soft size limit 5000 bytes and 100
				// character lines grouped into batches of
				// three with the same timestamp, we should
				// read the first 51 lines. This tests that the
				// reader accounts correctly for streams with
				// overlapping timestamps after hitting the
				// soft size limit.
				var mergedLines []LogLine
				for i := 0; i < 17; i++ {
					mergedLines = append(mergedLines, lines[i], lines[i], lines[i])
				}
				return mergedLines
			}(),
			formatLine: func(line LogLine) string { return line.Data + "\n" },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := NewLogIteratorReader(test.it, test.opts)

			var expectedData []byte
			for _, line := range test.expectedLines {
				expectedData = append(expectedData, []byte(test.formatLine(line))...)
			}

			var (
				nTotal   int
				readData []byte
			)
			// Use a small buffer when reading to ensure that there
			// is "left over" data between Read calls.
			p := make([]byte, 22)
			for {
				n, err := r.Read(p)
				nTotal += n
				readData = append(readData, p[:n]...)
				require.True(t, n >= 0)
				require.True(t, n <= len(p))
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
			assert.Equal(t, len(expectedData), nTotal)
			require.Equal(t, expectedData, readData)

			n, err := r.Read(p)
			assert.Zero(t, n)
			assert.Error(t, err)
		})
	}
	t.Run("EmptyBuffer", func(t *testing.T) {
		r := NewLogIteratorReader(
			newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket, chunks: chunks, parser: parser}),
			LogIteratorReaderOptions{},
		)

		p := make([]byte, 0)
		n, err := r.Read(p)
		assert.Zero(t, n)
		assert.NoError(t, err)
	})
	t.Run("ContextError", func(t *testing.T) {
		errCtx, errCancel := context.WithCancel(context.Background())
		errCancel()
		r := NewLogIteratorReader(
			newChunkIterator(errCtx, chunkIteratorOptions{bucket: bucket, chunks: chunks, parser: parser}),
			LogIteratorReaderOptions{},
		)

		p := make([]byte, 4096)
		_, err := r.Read(p)
		assert.Error(t, err)
	})
}
