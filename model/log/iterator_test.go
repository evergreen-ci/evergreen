package log

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	chunk   = "ChunkIterator"
	merging = "MergingIterator"
)

func TestLogIterator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tmpDir := t.TempDir()
	bucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: tmpDir})
	require.NoError(t, err)
	badBucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: tmpDir, Prefix: "DNE"})
	require.NoError(t, err)

	chunks, lines, parser, err := generateTestLog(ctx, bucket, 99, 30)
	require.NoError(t, err)

	for _, test := range []struct {
		name      string
		iterators map[string]LogIterator
		test      func(*testing.T, LogIterator)
	}{
		{
			name: "EmptyIterator",
			iterators: map[string]LogIterator{
				chunk:   newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket}),
				merging: newMergingIterator(newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket})),
			},
			test: func(t *testing.T, it LogIterator) {
				assert.False(t, it.Next())
				assert.True(t, it.Exhausted())
				assert.NoError(t, it.Err())
				assert.Zero(t, it.Item())
				assert.NoError(t, it.Close())
			},
		},
		{
			name: "ExhaustedIterator",
			iterators: map[string]LogIterator{
				chunk: newChunkIterator(ctx, chunkIteratorOptions{
					bucket: bucket,
					chunks: chunks,
					parser: parser,
				}),
				merging: newMergingIterator(newChunkIterator(ctx, chunkIteratorOptions{
					bucket: bucket,
					chunks: chunks,
					parser: parser,
				})),
			},
			test: func(t *testing.T, it LogIterator) {
				var count int
				for it.Next() {
					require.Less(t, count, len(lines))
					require.Equal(t, lines[count], it.Item())
					count++
				}
				assert.Equal(t, len(lines), count)
				assert.True(t, it.Exhausted())
				assert.NoError(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
		{
			name: "ErroredIterator",
			iterators: map[string]LogIterator{
				chunk: newChunkIterator(ctx, chunkIteratorOptions{
					bucket: badBucket,
					chunks: chunks,
					parser: parser,
				}),
				merging: newMergingIterator(newChunkIterator(ctx, chunkIteratorOptions{
					bucket: badBucket,
					chunks: chunks,
					parser: parser,
				})),
			},
			test: func(t *testing.T, it LogIterator) {
				assert.False(t, it.Next())
				assert.False(t, it.Exhausted())
				assert.Error(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for impl, it := range test.iterators {
				t.Run(impl, func(t *testing.T) {
					test.test(t, it)
				})
			}
		})
	}
}

func TestChunkIterator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: t.TempDir()})
	require.NoError(t, err)

	chunks, lines, parser, err := generateTestLog(ctx, bucket, 99, 30)
	require.NoError(t, err)

	for _, test := range []struct {
		name string
		opts chunkIteratorOptions
		test func(*testing.T, *chunkIterator)
	}{
		{
			name: "CorruptData",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: parser,
			},
			test: func(t *testing.T, it *chunkIterator) {
				chunks[0].numLines--
				defer func() {
					chunks[0].numLines++
				}()

				for it.Next() {
					// Exhaust.
				}
				assert.False(t, it.Exhausted())
				assert.Error(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
		{
			name: "ParsingError",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: func(_ string) (LogLine, error) { return LogLine{}, errors.New("parsing error") },
			},
			test: func(t *testing.T, it *chunkIterator) {
				assert.False(t, it.Next())
				assert.False(t, it.Exhausted())
				assert.Error(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
		{
			name: "GetChunkError",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: []chunkInfo{
					{
						key:      "DNE",
						numLines: 100,
						start:    time.Now().UTC().UnixNano() - int64(time.Hour),
						end:      time.Now().UTC().UnixNano(),
					},
				},
				parser: parser,
			},
			test: func(t *testing.T, it *chunkIterator) {
				assert.False(t, it.Next())
				assert.False(t, it.Exhausted())
				assert.Error(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
		{
			name: "SingleChunk",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: append([]chunkInfo{}, chunks[0]),
				parser: parser,
			},
			test: func(t *testing.T, it *chunkIterator) {
				var count int
				for it.Next() {
					require.Less(t, count, len(lines))
					require.Equal(t, lines[count], it.Item())
					count++
				}
				assert.True(t, it.Exhausted())
				assert.NoError(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
		{
			name: "Start",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: parser,
				start:  &chunks[1].start,
			},
			test: func(t *testing.T, it *chunkIterator) {
				offset := chunks[0].numLines
				var count int
				for it.Next() {
					require.Less(t, count+offset, len(lines))
					require.Equal(t, lines[count+offset], it.Item())
					count++
				}
				assert.Equal(t, len(lines)-chunks[0].numLines, count)
				assert.True(t, it.Exhausted())
				assert.NoError(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
		{
			name: "End",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: parser,
				end:    &chunks[1].start,
			},
			test: func(t *testing.T, it *chunkIterator) {
				var count int
				for it.Next() {
					require.Less(t, count, len(lines))
					require.Equal(t, lines[count], it.Item())
					count++
				}
				assert.Equal(t, chunks[0].numLines+1, count)
				assert.True(t, it.Exhausted())
				assert.NoError(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
		{
			name: "StartAndEnd",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: parser,
				start:  &chunks[1].start,
				end:    utility.ToInt64Ptr(chunks[len(chunks)-1].end - int64(time.Millisecond)),
			},
			test: func(t *testing.T, it *chunkIterator) {
				offset := chunks[0].numLines
				var count int
				for it.Next() {
					require.Less(t, count+offset, len(lines))
					require.Equal(t, lines[count+offset], it.Item())
					count++
				}
				assert.Equal(t, len(lines)-chunks[0].numLines-1, count)
				assert.True(t, it.Exhausted())
				assert.NoError(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
		{
			name: "LineLimit",
			opts: chunkIteratorOptions{
				bucket:    bucket,
				chunks:    chunks,
				parser:    parser,
				lineLimit: 35,
			},
			test: func(t *testing.T, it *chunkIterator) {
				var count int
				for it.Next() {
					require.Less(t, count, len(lines))
					require.Equal(t, lines[count], it.Item())
					count++
				}
				assert.Equal(t, 35, count)
				assert.True(t, it.Exhausted())
				assert.NoError(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
		{
			name: "TailN",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: parser,
				tailN:  20,
			},
			test: func(t *testing.T, it *chunkIterator) {
				offset := len(lines) - 20
				var count int
				for it.Next() {
					require.Less(t, count+offset, len(lines))
					require.Equal(t, lines[count+offset], it.Item())
					count++
				}
				assert.Equal(t, 20, count)
				assert.True(t, it.Exhausted())
				assert.NoError(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
		{
			name: "StartEndLineLimitTailN",
			opts: chunkIteratorOptions{
				bucket:    bucket,
				chunks:    chunks,
				parser:    parser,
				start:     &chunks[0].start,
				end:       &chunks[1].end,
				lineLimit: 5,
				tailN:     20,
			},
			test: func(t *testing.T, it *chunkIterator) {
				offset := chunks[0].numLines + chunks[1].numLines - 20
				var count int
				for it.Next() {
					require.Less(t, count+offset, len(lines))
					require.Equal(t, lines[count+offset], it.Item())
					count++
				}
				assert.Equal(t, 5, count)
				assert.True(t, it.Exhausted())
				assert.NoError(t, it.Err())
				assert.NoError(t, it.Close())
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.test(t, newChunkIterator(ctx, test.opts))
		})
	}
}

func TestMergingIterator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("SingleLog", func(t *testing.T) {
		bucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: t.TempDir()})
		require.NoError(t, err)
		chunks, lines, parser, err := generateTestLog(ctx, bucket, 100, 10)
		require.NoError(t, err)
		it := newMergingIterator(newChunkIterator(ctx, chunkIteratorOptions{
			bucket: bucket,
			chunks: chunks,
			parser: parser,
		}))

		var count int
		for it.Next() {
			require.Less(t, count, len(lines))
			require.Equal(t, lines[count], it.Item())
			count++
		}
		assert.Equal(t, len(lines), count)
		assert.NoError(t, it.Err())
		assert.NoError(t, it.Close())
	})
	t.Run("MultipleLogs", func(t *testing.T) {
		numLogs := 10
		its := make([]LogIterator, numLogs)
		lineMap := map[string]bool{}
		for i := 0; i < numLogs; i++ {
			bucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: t.TempDir()})
			require.NoError(t, err)
			chunks, lines, parser, err := generateTestLog(ctx, bucket, 100, 10)
			require.NoError(t, err)
			its[i] = newChunkIterator(ctx, chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: parser,
			})
			for _, line := range lines {
				lineMap[line.Data] = false
			}
		}
		it := newMergingIterator(its...)

		var (
			count         int
			lastTimestamp int64
		)
		for it.Next() {
			logLine := it.Item()
			require.LessOrEqual(t, lastTimestamp, logLine.Timestamp)
			seen, ok := lineMap[logLine.Data]
			require.True(t, ok)
			require.False(t, seen)

			lineMap[logLine.Data] = true
			lastTimestamp = logLine.Timestamp
			count++
		}
		assert.Len(t, lineMap, count)
		assert.NoError(t, it.Err())
		assert.NoError(t, it.Close())
	})
	t.Run("SomeExhausted", func(t *testing.T) {
		bucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: t.TempDir()})
		require.NoError(t, err)
		chunks, _, parser, err := generateTestLog(ctx, bucket, 100, 10)
		require.NoError(t, err)

		it0 := newChunkIterator(ctx, chunkIteratorOptions{
			bucket: bucket,
			chunks: chunks,
			parser: parser,
		})
		it1 := newChunkIterator(ctx, chunkIteratorOptions{
			bucket: bucket,
			chunks: chunks,
			parser: parser,
		})

		for it1.Next() {
			// Exhaust.
		}
		require.True(t, it1.Exhausted())
		require.NoError(t, it1.Err())

		it := newMergingIterator(it0, it1)
		assert.False(t, it.Exhausted())
		for it.Next() {
			// Exhaust.
		}
		assert.True(t, it.Exhausted())
		assert.NoError(t, it.Err())
		assert.NoError(t, it.Close())
		assert.NoError(t, it1.Close())
	})
}

// generateTestLog is a convenience function to generate random logs with 100
// character long lines of the given size and chunk size in the given bucket.
func generateTestLog(ctx context.Context, bucket pail.Bucket, size, chunkSize int) ([]chunkInfo, []LogLine, LineParser, error) {
	service := &logServiceV0{}

	lines := make([]LogLine, size)
	numChunks := size / chunkSize
	if numChunks == 0 || size%chunkSize > 0 {
		numChunks++
	}
	chunks := make([]chunkInfo, numChunks)
	ts := time.Now().UTC().UnixNano()
	logName := newRandCharSetString(16)

	for i := 0; i < numChunks; i++ {
		chunks[i] = chunkInfo{start: ts}

		var (
			lineCount int
			rawLines  string
		)
		for lineCount < chunkSize && lineCount+i*chunkSize < size {
			line := newRandCharSetString(100)
			lineNum := lineCount + i*chunkSize
			lines[lineNum] = LogLine{
				LogName:   logName,
				Priority:  level.Debug,
				Timestamp: ts,
				Data:      line,
			}
			rawLines += service.formatRawLine(lines[lineNum])
			ts += int64(time.Millisecond)
			lineCount++
		}

		chunks[i].end = ts - int64(time.Millisecond)
		chunks[i].numLines = lineCount
		chunks[i].key = logName + "/" + service.createChunkKey(chunks[i].start, chunks[i].end, chunks[i].numLines)

		if err := bucket.Put(ctx, chunks[i].key, strings.NewReader(rawLines)); err != nil {
			return []chunkInfo{}, []LogLine{}, nil, errors.Wrap(err, "adding chunk to bucket")
		}

		ts += int64(time.Hour)
	}

	return chunks, lines, service.getParser(logName), nil
}

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

func newRandCharSetString(length int) string {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}
