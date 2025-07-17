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

			t.Run("Append", func(t *testing.T) {
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
					{
						Priority:  level.Info,
						Timestamp: time.Now().UnixNano(),
						Data:      "",
					},
					{
						Priority:  level.Info,
						Timestamp: time.Now().UnixNano(),
						Data:      "\n",
					},
				}
				require.NoError(t, svc.Append(ctx, logName, 1, lines))

				foundLines := readLogLines(t, svc, ctx, GetOptions{LogNames: []string{logName}})
				require.Len(t, foundLines, len(lines))

				ts := time.Now().UnixNano()
				offset := len(lines)
				lines = append(
					lines,
					LogLine{
						Priority:  level.Warning,
						Timestamp: ts,
						Data:      "Appending a warning line to an already existing log.",
					},
					LogLine{
						Priority:  level.Error,
						Timestamp: ts,
						Data:      "Appending an error line to an already existing log.",
					},
				)
				require.NoError(t, svc.Append(ctx, logName, 1, lines[offset:]))

				offset = len(lines)
				lines = append(
					lines,
					LogLine{
						Priority:  level.Warning,
						Timestamp: ts,
						Data:      "Appending another chunk of logs with the same time range as the previous chunk.",
					},
					LogLine{
						Priority:  level.Error,
						Timestamp: ts,
						Data:      "If two chunks have the same time range, the order of lines should appear in the order they were uploaded.",
					},
				)
				require.NoError(t, svc.Append(ctx, logName, 1, lines[offset:]))

				lines = append(
					[]LogLine{
						{
							Priority:  level.Warning,
							Timestamp: time.Now().Add(-time.Minute).UnixNano(),
							Data:      "This should be the first logline.",
						},
						{
							Priority:  level.Error,
							Timestamp: time.Now().Add(-time.Minute).UnixNano(),
							Data:      "Even though these sequence chunk was added later, it should be read before the previous chunk.",
						},
					},
					lines...,
				)
				require.NoError(t, svc.Append(ctx, logName, 0, lines[0:2]))

				foundLines = readLogLines(t, svc, ctx, GetOptions{LogNames: []string{logName}})
				require.Len(t, foundLines, len(lines))
				for i := 0; i < len(lines); i++ {
					assert.Equal(t, logName, foundLines[i].LogName)
					assert.Equal(t, lines[i].Priority, foundLines[i].Priority)
					assert.Equal(t, lines[i].Timestamp, foundLines[i].Timestamp)
					assert.Equal(t, strings.TrimRight(lines[i].Data, "\n"), foundLines[i].Data)
				}
			})
			t.Run("Get", func(t *testing.T) {
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
						Data:      "Another lonely log line.",
					},
				}
				require.NoError(t, svc.Append(ctx, log0, 0, lines0))
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
				require.NoError(t, svc.Append(ctx, log1, 0, lines1))
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
				require.NoError(t, svc.Append(ctx, log2, 0, lines2))

				for _, test := range []struct {
					name          string
					opts          GetOptions
					expectedLines []LogLine
				}{
					{
						name: "LogDNE",
						opts: GetOptions{LogNames: []string{"DNE"}},
					},
					{
						name: "CommonPrefix",
						opts: GetOptions{LogNames: []string{log0, "common"}},
						expectedLines: []LogLine{
							lines1[0],
							lines0[0],
							lines2[0],
							lines2[1],
							lines0[1],
							lines1[1],
							lines2[2],
						},
					},
					{
						name: "DefaultTimeRangeOfFirstLog",
						opts: GetOptions{
							LogNames:                   []string{log0, log1},
							DefaultTimeRangeOfFirstLog: true,
						},
						expectedLines: []LogLine{
							lines1[0],
							lines0[0],
							lines0[1],
						},
					},
					{
						name: "DefaultTimeRangeOfFirstLogWithStart",
						opts: GetOptions{
							LogNames:                   []string{log2, log1},
							Start:                      utility.ToInt64Ptr(ts + int64(time.Second)),
							DefaultTimeRangeOfFirstLog: true,
						},
						expectedLines: []LogLine{
							lines2[0],
							lines2[1],
							lines1[1],
							lines2[2],
						},
					},
					{
						name: "DefaultTimeRangeOfFirstLogWithEnd",
						opts: GetOptions{
							LogNames:                   []string{log2, log1},
							End:                        utility.ToInt64Ptr(ts + int64(time.Minute)),
							DefaultTimeRangeOfFirstLog: true,
						},
						expectedLines: []LogLine{
							lines2[0],
							lines2[1],
							lines1[1],
						},
					},
					{
						name: "StartSingleLog",
						opts: GetOptions{
							LogNames: []string{log1},
							Start:    utility.ToInt64Ptr(ts + int64(time.Second)),
						},
						expectedLines: []LogLine{lines1[1]},
					},
					{
						name: "StartMultipleLogs",
						opts: GetOptions{
							LogNames: []string{log0, "common"},
							Start:    utility.ToInt64Ptr(ts + int64(time.Second)),
						},
						expectedLines: []LogLine{
							lines2[0],
							lines2[1],
							lines0[1],
							lines1[1],
							lines2[2],
						},
					},
					{
						name: "EndSingleLog",
						opts: GetOptions{
							LogNames: []string{log2},
							End:      utility.ToInt64Ptr(ts + int64(time.Minute)),
						},
						expectedLines: []LogLine{
							lines2[0],
							lines2[1],
						},
					},
					{
						name: "EndMultipleLogs",
						opts: GetOptions{
							LogNames: []string{log0, "common"},
							End:      utility.ToInt64Ptr(ts + int64(time.Second)),
						},
						expectedLines: []LogLine{
							lines1[0],
							lines0[0],
							lines2[0],
							lines2[1],
						},
					},
					{
						name: "StartAndEndSingleLog",
						opts: GetOptions{
							LogNames:                   []string{log2},
							Start:                      utility.ToInt64Ptr(ts + int64(time.Second)),
							End:                        utility.ToInt64Ptr(ts + int64(time.Minute)),
							DefaultTimeRangeOfFirstLog: true, // Should get ignored.
						},
						expectedLines: []LogLine{
							lines2[0],
							lines2[1],
						},
					},
					{
						name: "StartAndEndMultipleLogs",
						opts: GetOptions{
							LogNames:                   []string{log1, log2},
							Start:                      utility.ToInt64Ptr(ts + int64(time.Second)),
							End:                        utility.ToInt64Ptr(ts + int64(time.Minute)),
							DefaultTimeRangeOfFirstLog: true, // Should get ignored.
						},
						expectedLines: []LogLine{
							lines2[0],
							lines2[1],
							lines1[1],
						},
					},
					{
						name: "LineLimitSingleLog",
						opts: GetOptions{
							LogNames:  []string{log2},
							LineLimit: 2,
						},
						expectedLines: []LogLine{
							lines2[0],
							lines2[1],
						},
					},
					{
						name: "LineLimitMultipleLogs",
						opts: GetOptions{
							LogNames:  []string{log0, "common"},
							LineLimit: 4,
						},
						expectedLines: []LogLine{
							lines1[0],
							lines0[0],
							lines2[0],
							lines2[1],
						},
					},
					{
						name: "TailNSingleLog",
						opts: GetOptions{
							LogNames: []string{log2},
							TailN:    2,
						},
						expectedLines: []LogLine{
							lines2[1],
							lines2[2],
						},
					},
					{
						name: "TailNMultipleLogs",
						opts: GetOptions{
							LogNames: []string{log0, "common"},
							TailN:    3,
						},
						expectedLines: []LogLine{
							lines0[1],
							lines1[1],
							lines2[2],
						},
					},
				} {
					t.Run(test.name, func(t *testing.T) {
						actualLines := readLogLines(t, svc, ctx, test.opts)
						assert.Equal(t, test.expectedLines, actualLines)
					})
				}
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
