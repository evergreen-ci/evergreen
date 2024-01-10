package log

import (
	"context"
	"math/rand"
	"sort"
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
	basic   = "BasicIterator"
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
				basic:   newBasicIterator(nil),
				chunk:   newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket}),
				merging: newMergingIterator(0, newChunkIterator(ctx, chunkIteratorOptions{bucket: bucket})),
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
				basic: newBasicIterator(lines),
				chunk: newChunkIterator(ctx, chunkIteratorOptions{
					bucket: bucket,
					chunks: chunks,
					parser: parser,
				}),
				merging: newMergingIterator(0, newChunkIterator(ctx, chunkIteratorOptions{
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
				merging: newMergingIterator(0, newChunkIterator(ctx, chunkIteratorOptions{
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
		name          string
		opts          chunkIteratorOptions
		expectedLines []LogLine
		hasErr        bool
	}{
		{
			name: "CorruptData",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: append(
					[]chunkInfo{
						{
							key:      chunks[0].key,
							numLines: chunks[0].numLines - 1,
							start:    chunks[0].start,
							end:      chunks[0].end,
						},
					},
					chunks[1:len(chunks)-1]...,
				),
				parser: parser,
			},
			hasErr: true,
		},
		{
			name: "ParsingError",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: func(_ string) (LogLine, error) { return LogLine{}, errors.New("parsing error") },
			},
			hasErr: true,
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
			hasErr: true,
		},
		{
			name: "SingleChunk",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: append([]chunkInfo{}, chunks[0]),
				parser: parser,
			},
			expectedLines: lines[:chunks[0].numLines],
		},
		{
			name: "Start",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: parser,
				start:  &chunks[1].start,
			},
			expectedLines: lines[chunks[0].numLines:],
		},
		{
			name: "End",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: parser,
				end:    &chunks[1].start,
			},
			expectedLines: lines[:chunks[0].numLines+1],
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
			expectedLines: lines[chunks[0].numLines : len(lines)-1],
		},
		{
			name: "LineLimitLessThanLineCount",
			opts: chunkIteratorOptions{
				bucket:    bucket,
				chunks:    chunks,
				parser:    parser,
				lineLimit: 35,
			},
			expectedLines: lines[:35],
		},
		{
			name: "LineLimitEqualToLineCount",
			opts: chunkIteratorOptions{
				bucket:    bucket,
				chunks:    chunks,
				parser:    parser,
				lineLimit: len(lines),
			},
			expectedLines: lines,
		},
		{
			name: "LineLimitGreaterThanLineCount",
			opts: chunkIteratorOptions{
				bucket:    bucket,
				chunks:    chunks,
				parser:    parser,
				lineLimit: len(lines),
			},
			expectedLines: lines,
		},
		{
			name: "TailNLessThanLineCount",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: parser,
				tailN:  20,
			},
			expectedLines: lines[len(lines)-20:],
		},
		{
			name: "TailNEqualToLineCount",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: parser,
				tailN:  len(lines),
			},
			expectedLines: lines,
		},
		{
			name: "TailNGreaterThanLineCount",
			opts: chunkIteratorOptions{
				bucket: bucket,
				chunks: chunks,
				parser: parser,
				tailN:  2 * len(lines),
			},
			expectedLines: lines,
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
			expectedLines: lines[chunks[0].numLines+chunks[1].numLines-20 : chunks[0].numLines+chunks[1].numLines-15],
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			it := newChunkIterator(ctx, test.opts)
			assert.False(t, it.Exhausted())

			var actualLines []LogLine
			for it.Next() {
				actualLines = append(actualLines, it.Item())
			}

			if test.hasErr {
				assert.False(t, it.Exhausted())
				assert.Error(t, it.Err())
				assert.NoError(t, it.Close())
			} else {
				require.NoError(t, it.Err())
				assert.True(t, it.Exhausted())
				assert.NoError(t, it.Close())
				assert.Equal(t, test.expectedLines, actualLines)
			}
		})
	}
}

func TestMergingIterator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: t.TempDir()})
	require.NoError(t, err)

	logs := make([][]LogLine, 2)
	var mergedLines []LogLine
	for i := 0; i < 2; i++ {
		_, lines, _, err := generateTestLog(ctx, nil, 100, 10)
		require.NoError(t, err)

		logs[i] = lines
		mergedLines = append(mergedLines, lines...)
	}
	sort.SliceStable(mergedLines, func(i, j int) bool {
		return mergedLines[i].Timestamp < mergedLines[j].Timestamp
	})

	generateIterators := func(logs ...[]LogLine) []LogIterator {
		its := make([]LogIterator, len(logs))
		for i := 0; i < len(logs); i++ {
			its[i] = newBasicIterator(logs[i])
		}

		return its
	}

	for _, test := range []struct {
		name          string
		lineLimit     int
		its           []LogIterator
		expectedLines []LogLine
		hasErr        bool
	}{
		{
			name: "InitError",
			its: func() []LogIterator {
				chunks, _, _, err := generateTestLog(ctx, bucket, 100, 10)
				require.NoError(t, err)
				it := newChunkIterator(ctx, chunkIteratorOptions{
					bucket: bucket,
					chunks: chunks,
					parser: func(_ string) (LogLine, error) { return LogLine{}, errors.New("parsing error") },
				})

				return append(generateIterators(logs...), it)
			}(),
			hasErr: true,
		},
		{
			name: "NextError",
			its: func() []LogIterator {
				chunks, _, parser, err := generateTestLog(ctx, bucket, 100, 10)
				require.NoError(t, err)
				chunks[0].numLines--
				it := newChunkIterator(ctx, chunkIteratorOptions{
					bucket: bucket,
					chunks: chunks,
					parser: parser,
				})

				return append(generateIterators(logs...), it)
			}(),
			hasErr: true,
		},
		{
			name:          "SingleLog",
			its:           generateIterators(logs[0]),
			expectedLines: logs[0],
		},
		{
			name:          "MultipleLogs",
			its:           generateIterators(logs...),
			expectedLines: mergedLines,
		},
		{
			name:          "LineLimitLessThanLineCount",
			lineLimit:     10,
			its:           generateIterators(logs...),
			expectedLines: mergedLines[:10],
		},
		{
			name:          "LineLimitEqualToLineCount",
			lineLimit:     len(mergedLines),
			its:           generateIterators(logs...),
			expectedLines: mergedLines,
		},
		{
			name:          "LineLimitGreaterThanLineCount",
			lineLimit:     2 * len(mergedLines),
			its:           generateIterators(logs...),
			expectedLines: mergedLines,
		},
		{
			name: "SomeExhausted",
			its: func() []LogIterator {
				its := generateIterators(logs[0], logs[1])
				for its[0].Next() {
					// Exhaust.
				}

				return its
			}(),
			expectedLines: logs[1],
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			it := newMergingIterator(test.lineLimit, test.its...)
			assert.False(t, it.Exhausted())

			var actualLines []LogLine
			for it.Next() {
				actualLines = append(actualLines, it.Item())
			}

			if test.hasErr {
				assert.False(t, it.Exhausted())
				assert.Error(t, it.Err())
				assert.NoError(t, it.Close())
			} else {
				require.NoError(t, it.Err())
				assert.True(t, it.Exhausted())
				assert.NoError(t, it.Close())
				assert.Equal(t, test.expectedLines, actualLines)
			}
		})
	}
}

func TestNewTailIterator(t *testing.T) {
	_, lines, _, err := generateTestLog(context.TODO(), nil, 100, 10)
	require.NoError(t, err)

	for _, test := range []struct {
		name          string
		n             int
		expectedLines []LogLine
	}{
		{
			name:          "TailLessThanTotalLineCount",
			n:             22,
			expectedLines: lines[len(lines)-22:],
		},
		{
			name:          "TailEqualToTotalLineCount",
			n:             len(lines),
			expectedLines: lines,
		},
		{
			name:          "TailGreaterThanTotalLineCount",
			n:             2 * len(lines),
			expectedLines: lines,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			it, err := newTailIterator(newBasicIterator(lines), test.n)
			require.NoError(t, err)
			assert.False(t, it.Exhausted())

			var actualLines []LogLine
			for it.Next() {
				actualLines = append(actualLines, it.Item())
			}
			require.NoError(t, it.Err())
			assert.NoError(t, it.Close())

			assert.Equal(t, test.expectedLines, actualLines)
		})
	}
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

		if bucket != nil {
			if err := bucket.Put(ctx, chunks[i].key, strings.NewReader(rawLines)); err != nil {
				return []chunkInfo{}, []LogLine{}, nil, errors.Wrap(err, "adding chunk to bucket")
			}
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
