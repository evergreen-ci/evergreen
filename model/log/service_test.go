package log

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, impl := range []struct {
		name        string
		constructor func(*testing.T) LogService
	}{
		{
			name: "V0",
			constructor: func(t *testing.T) LogService {
				bucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: t.TempDir()})
				require.NoError(t, err)

				return &logServiceV0{
					bucket: bucket,
				}
			},
		},
	} {
		t.Run(impl.name, func(t *testing.T) {
			svc := impl.constructor(t)

			t.Run("LogDNE", func(t *testing.T) {
				lines := readLogLines(t, svc, ctx, GetOptions{LogNames: []string{"DNE"}})
				assert.Empty(t, lines)
			})
			t.Run("RoundTripSingleLog", func(t *testing.T) {
				logName := utility.RandomString()
				lines := []LogLine{
					{
						Priority:  level.Debug,
						Timestamp: time.Now().UnixNano(),
						Data:      "This is a log line.",
					},
					{
						Priority:  level.Info,
						Timestamp: time.Now().UnixNano(),
						Data:      "The new line at the end of this line should be handled properly.\n",
					},
				}
				require.NoError(t, svc.Append(ctx, logName, lines))

				foundLines := readLogLines(t, svc, ctx, GetOptions{LogNames: []string{logName}})
				require.Len(t, foundLines, len(lines))

				offset := len(lines)
				lines = append(
					lines,
					LogLine{
						Priority:  level.Warning,
						Timestamp: time.Now().UnixNano(),
						Data:      "Appending a warning line to an already existing log.",
					},
					LogLine{
						Priority:  level.Error,
						Timestamp: time.Now().UnixNano(),
						Data:      "Appending an error line to an already existing log.",
					},
				)
				require.NoError(t, svc.Append(ctx, logName, lines[offset:]))

				foundLines = readLogLines(t, svc, ctx, GetOptions{LogNames: []string{logName}})
				require.Len(t, foundLines, len(lines))
				for i := 0; i < len(lines); i++ {
					assert.Equal(t, logName, foundLines[i].LogName)
					assert.Equal(t, lines[i].Priority, foundLines[i].Priority)
					assert.Equal(t, lines[i].Timestamp, foundLines[i].Timestamp)
					assert.Equal(t, strings.TrimRight(lines[i].Data, "\n"), foundLines[i].Data)
				}
			})
			t.Run("RoundTripMultipleLogs", func(t *testing.T) {
				ts := time.Now().UnixNano()
				log0 := "lonely/alone.log"
				lines0 := []LogLine{
					{
						LogName:   log0,
						Priority:  level.Info,
						Timestamp: ts,
						Data:      "Alone in my prefix.",
					},
					{
						LogName:   log0,
						Priority:  level.Info,
						Timestamp: ts + int64(30*time.Second),
						Data:      "Another lonley log line.",
					},
				}
				require.NoError(t, svc.Append(ctx, log0, lines0))
				log1 := "common/1.log"
				lines1 := []LogLine{
					{
						LogName:   log1,
						Priority:  level.Debug,
						Timestamp: ts,
						Data:      "Part of the common prefix.",
					},
					{
						LogName:   log1,
						Priority:  level.Error,
						Timestamp: ts + int64(time.Minute),
						Data:      "Another line here.",
					},
				}
				require.NoError(t, svc.Append(ctx, log1, lines1))
				log2 := "common/2.log"
				lines2 := []LogLine{
					{
						LogName:   log2,
						Priority:  level.Debug,
						Timestamp: ts + int64(time.Second),
						Data:      "Also part of the common prefix.",
					},
					{
						LogName:   log2,
						Priority:  level.Debug,
						Timestamp: ts + int64(time.Second),
						Data:      "I have the same timestamp as the last line.",
					},
					{
						LogName:   log2,
						Priority:  level.Debug,
						Timestamp: ts + int64(time.Hour),
						Data:      "And this line comes way later.",
					},
				}
				require.NoError(t, svc.Append(ctx, log2, lines2))

				t.Run("CommonPrefix", func(t *testing.T) {
					expectedLines := []LogLine{
						lines1[0],
						lines0[0],
						lines2[0],
						lines2[1],
						lines0[1],
						lines1[1],
						lines2[2],
					}
					actualLines := readLogLines(t, svc, ctx, GetOptions{LogNames: []string{log0, "common"}})
					assert.Equal(t, expectedLines, actualLines)
				})
				t.Run("DefaultTimeRangeOfFirstLog", func(t *testing.T) {
					expectedLines := []LogLine{
						lines1[0],
						lines0[0],
						lines0[1],
					}
					actualLines := readLogLines(t, svc, ctx, GetOptions{
						LogNames:                   []string{log0, log1},
						DefaultTimeRangeOfFirstLog: true,
					})
					assert.Equal(t, expectedLines, actualLines)
				})
				t.Run("StartAndEnd", func(t *testing.T) {
					expectedLines := []LogLine{
						lines2[0],
						lines2[1],
						lines1[1],
					}
					actualLines := readLogLines(t, svc, ctx, GetOptions{
						LogNames:                   []string{log1, log2},
						Start:                      utility.ToInt64Ptr(ts + int64(time.Second)),
						End:                        utility.ToInt64Ptr(ts + int64(time.Minute)),
						DefaultTimeRangeOfFirstLog: true,
					})
					assert.Equal(t, expectedLines, actualLines)
				})
				t.Run("LineLimit", func(t *testing.T) {
					expectedLines := []LogLine{
						lines1[0],
						lines0[0],
						lines2[0],
						lines2[1],
					}
					actualLines := readLogLines(t, svc, ctx, GetOptions{
						LogNames:  []string{log0, "common"},
						LineLimit: 4,
					})
					assert.Equal(t, expectedLines, actualLines)
				})
				t.Run("TailN", func(t *testing.T) {
					expectedLines := []LogLine{
						lines0[1],
						lines1[1],
						lines2[2],
					}
					actualLines := readLogLines(t, svc, ctx, GetOptions{
						LogNames: []string{log0, "common"},
						TailN:    3,
					})
					assert.Equal(t, expectedLines, actualLines)
				})
			})
		})
	}
}

func readLogLines(t *testing.T, svc LogService, ctx context.Context, opts GetOptions) []LogLine {
	var lines []LogLine
	it, err := svc.Get(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, it)
	for it.Next() {
		lines = append(lines, it.Item())
	}
	require.NoError(t, it.Err())
	assert.NoError(t, it.Close())

	return lines

}
