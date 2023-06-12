package log

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/evergreen-ci/pail"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

type logServiceV0 struct {
	bucket pail.Bucket
}

func (s *logServiceV0) GetTaskLogPrefix(opts TaskOptions, logType TaskLogType) (string, error) {
	prefix := fmt.Sprintf("task_id=%s/execution=%d", opts.TaskID, opts.Execution)

	switch logType {
	case TaskLogTypeAll:
		prefix += "/task_logs"
	case TaskLogTypeAgent:
		prefix += "/task_logs/agent"
	case TaskLogTypeTask:
		prefix += "/task_logs/task"
	case TaskLogTypeSystem:
		prefix += "/task_logs/system"
	case TaskLogTypeTest:
		prefix = "/test_logs"
	default:
		return "", errors.Errorf("unsupported task log type '%s'", logType)
	}

	return prefix, nil
}

func (s *logServiceV0) GetTaskLogs(ctx context.Context, prefix string, getOpts GetOptions) (LogIterator, error) {
	chunkGroups, err := s.getChunkGroups(ctx, prefix)
	if err != nil {
		return nil, errors.Wrap(err, "getting log chunks")
	}

	var its []LogIterator
	for _, chunkGroup := range chunkGroups {
		its = append(its, newChunkIterator(ctx, chunkIteratorOptions{
			bucket:    s.bucket,
			chunks:    chunkGroup,
			parser:    s.getParser(),
			start:     getOpts.Start,
			end:       getOpts.End,
			lineLimit: getOpts.LineLimit,
			tailN:     getOpts.TailN,
		}))
	}

	if len(chunkGroups) == 1 {
		return its[0], nil
	}
	return newMergingIterator(its...), nil
}

// getChunkGroups maps each logical log to its chunk files stored in
// pail-backed bucket storage for the given prefix.
func (s *logServiceV0) getChunkGroups(ctx context.Context, prefix string) (map[string][]chunkInfo, error) {
	chunkGroups := map[string][]chunkInfo{}

	it, err := s.bucket.List(ctx, prefix)
	if err != nil {
		return nil, errors.Wrap(err, "listing log chunks")
	}
	for it.Next(ctx) {
		chunk, err := s.parseChunkKey(it.Item().Name())
		if err != nil {
			return nil, errors.Wrapf(err, "parsing chunk key '%s'", it.Item().Name())
		}
		chunkGroups[chunk.prefix] = append(chunkGroups[chunk.prefix], chunk)
	}
	if err = it.Err(); err != nil {
		return nil, errors.Wrap(err, "iterating log chunks")
	}

	for _, chunkGroup := range chunkGroups {
		sort.Slice(chunkGroup, func(i, j int) bool {
			return chunkGroup[i].start > chunkGroup[j].start
		})
	}

	return chunkGroups, nil
}

// createChunkKey returns a pail-backed bucket storage key that encodes the
// given log chunk information. This is used primarily for fetching logs.
func (s *logServiceV0) createChunkKey(start, end int64, numLines int) string {
	return fmt.Sprintf("%d_%d_%d", start, end, numLines)
}

// parseChunkKey returns a chunkInfo object with the information encoded in the
// given key.
func (s *logServiceV0) parseChunkKey(key string) (chunkInfo, error) {
	var prefix string
	if lastIdx := strings.LastIndex(key, "/"); lastIdx >= 0 {
		prefix = key[:lastIdx]
		key = key[lastIdx+1:]
	}

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
		prefix:   prefix,
		key:      key,
		numLines: numLines,
		start:    start,
		end:      end,
	}, nil
}

// formatRawLing returns a log line in the raw storage format.
func (s *logServiceV0) formatRawLine(line LogLine) string {
	return fmt.Sprintf("%d %d %s", line.Priority, line.Timestamp, line.Data)
}

// getParser returns a function that parses a raw v0 log line into a LogLine
// struct.
func (s *logServiceV0) getParser() lineParser {
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
			Priority:  level.Priority(priority),
			Timestamp: ts,
			Data:      lineParts[2],
		}, nil
	}
}
