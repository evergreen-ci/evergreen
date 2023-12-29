package log

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/evergreen-ci/pail"
	"github.com/jpillora/longestcommon"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

type logServiceV0 struct {
	bucket pail.Bucket
}

// NewLogServiceV0 returns a new V0 Evergreen log service.
func NewLogServiceV0(bucket pail.Bucket) *logServiceV0 {
	return &logServiceV0{bucket: bucket}
}

func (s *logServiceV0) Get(ctx context.Context, getOpts GetOptions) (LogIterator, error) {
	var its []LogIterator
	logChunks, firstStart, firstEnd, err := s.getLogChunks(ctx, getOpts.LogNames)
	if err != nil {
		return nil, errors.Wrap(err, "getting log chunks")
	}

	start, end := getOpts.Start, getOpts.End
	if getOpts.DefaultTimeRangeOfFirstLog && len(getOpts.LogNames) > 1 {
		if start == nil {
			start = &firstStart
		}
		if end == nil {
			end = &firstEnd
		}
	}

	for name, chunks := range logChunks {
		its = append(its, newChunkIterator(ctx, chunkIteratorOptions{
			bucket:    s.bucket,
			chunks:    chunks,
			parser:    s.getParser(name),
			start:     start,
			end:       end,
			lineLimit: getOpts.LineLimit,
			tailN:     getOpts.TailN,
		}))
	}

	if len(its) == 1 {
		return its[0], nil
	}
	return newMergingIterator(its...), nil
}

func (s *logServiceV0) Append(ctx context.Context, logName string, lines []LogLine) error {
	if len(lines) == 0 {
		return nil
	}

	var rawLines []byte
	for _, line := range lines {
		rawLines = append(rawLines, []byte(s.formatRawLine(line))...)
	}

	key := fmt.Sprintf("%s/%s", logName, s.createChunkKey(lines[0].Timestamp, lines[len(lines)-1].Timestamp, len(lines)))
	return errors.Wrap(s.bucket.Put(ctx, key, bytes.NewReader(rawLines)), "writing log chunk to bucket")
}

// getLogChunks maps each logical log to its chunk files stored in pail-backed
// bucket storage for the given prefix.
func (s *logServiceV0) getLogChunks(ctx context.Context, logNames []string) (map[string][]chunkInfo, int64, int64, error) {
	logChunks := map[string][]chunkInfo{}

	// To reduce potentially expensive list calls, use the LCP of the
	// given log names when calling `bucket.List`. Key names that do not
	// have one of the log names as a prefix will get filtered out.
	prefix := longestcommon.Prefix(logNames)
	match := func(key string) bool {
		for _, name := range logNames {
			if strings.HasPrefix(key, name) {
				return true
			}
		}

		return false
	}

	it, err := s.bucket.List(ctx, prefix)
	if err != nil {
		return nil, 0, 0, errors.Wrap(err, "listing log chunks")
	}
	for it.Next(ctx) {
		logName := prefix
		chunkKey := it.Item().Name()

		if !match(chunkKey) {
			continue
		}

		// Strip any prefix from the key and set it as the log's name;
		// callers may pass in prefixes that contain multiple logical
		// logs.
		if lastIdx := strings.LastIndex(chunkKey, "/"); lastIdx >= 0 {
			logName = chunkKey[:lastIdx]
			chunkKey = chunkKey[lastIdx+1:]
		}

		chunk, err := s.parseChunkKey(logName, chunkKey)
		if err != nil {
			return nil, 0, 0, errors.Wrapf(err, "parsing chunk key '%s'", chunkKey)
		}
		logChunks[logName] = append(logChunks[logName], chunk)
	}
	if err = it.Err(); err != nil {
		return nil, 0, 0, errors.Wrap(err, "iterating log chunks")
	}

	var start, end int64
	for key, chunks := range logChunks {
		sort.Slice(chunks, func(i, j int) bool {
			return chunks[i].start < chunks[j].start
		})
		if strings.HasPrefix(key, logNames[0]) {
			if start == 0 || (start > 0 && start > chunks[0].start) {
				start = chunks[0].start
			}
			if end < chunks[len(chunks)-1].end {
				end = chunks[len(chunks)-1].end
			}
		}
	}

	return logChunks, start, end, nil
}

// createChunkKey returns a pail-backed bucket storage key that encodes the
// given log chunk information. This is used primarily for fetching logs.
func (s *logServiceV0) createChunkKey(start, end int64, numLines int) string {
	return fmt.Sprintf("%d_%d_%d", start, end, numLines)
}

// parseChunkKey returns a chunkInfo object with the information encoded in the
// given key.
func (s *logServiceV0) parseChunkKey(prefix, key string) (chunkInfo, error) {
	parsedKey := strings.Split(key, "_")
	if len(parsedKey) != 3 {
		return chunkInfo{}, errors.New("invalid key format")
	}

	start, err := strconv.ParseInt(parsedKey[0], 10, 64)
	if err != nil {
		return chunkInfo{}, errors.Wrap(err, "parsing start time")
	}
	end, err := strconv.ParseInt(parsedKey[1], 10, 64)
	if err != nil {
		return chunkInfo{}, errors.Wrap(err, "parsing end time")
	}
	numLines, err := strconv.Atoi(parsedKey[2])
	if err != nil {
		return chunkInfo{}, errors.Wrap(err, "parsing num lines")
	}

	return chunkInfo{
		key:      prefix + "/" + key,
		numLines: numLines,
		start:    start,
		end:      end,
	}, nil
}

// formatRawLine formats a log line for storage.
func (s *logServiceV0) formatRawLine(line LogLine) string {
	if line.Data[len(line.Data)-1] != '\n' {
		line.Data += "\n"
	}

	return fmt.Sprintf("%d %d %s", line.Priority, line.Timestamp, line.Data)
}

// getParser returns a function that parses a raw v0 log line into a LogLine
// struct.
func (s *logServiceV0) getParser(logName string) LineParser {
	return func(data string) (LogLine, error) {
		lineParts := strings.SplitN(data, " ", 3)
		if len(lineParts) != 3 {
			return LogLine{}, errors.New("malformed log line")
		}

		priority, err := strconv.ParseInt(strings.TrimSpace(lineParts[0]), 10, 16)
		if err != nil {
			return LogLine{}, err
		}

		ts, err := strconv.ParseInt(lineParts[1], 10, 64)
		if err != nil {
			return LogLine{}, err
		}

		return LogLine{
			LogName:   logName,
			Priority:  level.Priority(priority),
			Timestamp: ts,
			Data:      strings.TrimSuffix(lineParts[2], "\n"),
		}, nil
	}
}
