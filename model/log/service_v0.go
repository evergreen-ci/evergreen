package log

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/pail"
	"github.com/jpillora/longestcommon"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

// logServiceV0 implements a pail-backed log service for Evergreen.
type logServiceV0 struct {
	bucket pail.Bucket
}

// NewLogServiceV0 returns a new V0 Evergreen log service.
func NewLogServiceV0(bucket pail.Bucket) *logServiceV0 {
	return &logServiceV0{bucket: bucket}
}

func (s *logServiceV0) Get(ctx context.Context, getOpts GetOptions) (LogIterator, error) {
	allLogChunks, firstStart, firstEnd, err := s.getLogChunks(ctx, getOpts.LogNames)
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

	var its []LogIterator
	for _, chunks := range allLogChunks {
		its = append(its, newChunkIterator(ctx, chunkIteratorOptions{
			bucket:    s.bucket,
			chunks:    chunks.chunks,
			parser:    s.getParser(chunks.name),
			start:     start,
			end:       end,
			lineLimit: getOpts.LineLimit,
			tailN:     getOpts.TailN,
		}))
	}

	if len(its) == 1 {
		return its[0], nil
	}

	it := newMergingIterator(getOpts.LineLimit, its...)
	if getOpts.TailN > 0 {
		return newTailIterator(it, getOpts.TailN)
	}
	return it, nil
}

func (s *logServiceV0) Append(ctx context.Context, logName string, sequence int, lines []LogLine) error {
	if len(lines) == 0 {
		return nil
	}

	var rawLines []byte
	for _, line := range lines {
		rawLines = append(rawLines, []byte(s.formatRawLine(line))...)
	}

	key := fmt.Sprintf("%s/%s", logName, s.createChunkKey(sequence, lines[0].Timestamp, lines[len(lines)-1].Timestamp, len(lines)))
	return errors.Wrap(s.bucket.Put(ctx, key, bytes.NewReader(rawLines)), "writing log chunk to bucket")
}

// getLogChunks maps each logical log to its chunk files stored in pail-backed
// bucket storage for the given prefix.
func (s *logServiceV0) getLogChunks(ctx context.Context, logNames []string) ([]chunkGroup, int64, int64, error) {
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

	var orderedLogNames []string
	logChunks := map[string][]chunkInfo{}
	for it.Next(ctx) {
		chunkKey := it.Item().Name()
		if !match(chunkKey) {
			continue
		}

		// Strip any prefix from the key and set it as the log's name;
		// callers may pass in prefixes that contain multiple logical
		// logs.
		logName := prefix
		if lastIdx := strings.LastIndex(chunkKey, "/"); lastIdx >= 0 {
			logName = chunkKey[:lastIdx]
			chunkKey = chunkKey[lastIdx+1:]
		}

		chunk, err := s.parseChunkKey(logName, chunkKey)
		if err != nil {
			return nil, 0, 0, errors.Wrapf(err, "parsing chunk key '%s'", chunkKey)
		}

		if _, ok := logChunks[logName]; !ok {
			orderedLogNames = append(orderedLogNames, logName)
		}
		logChunks[logName] = append(logChunks[logName], chunk)
	}
	if err = it.Err(); err != nil {
		return nil, 0, 0, errors.Wrap(err, "iterating log chunks")
	}

	var start, end int64
	for name, chunks := range logChunks {
		// Sort each set of chunks by start order for log iterating and
		// find the first specified log's time range.
		sort.Slice(chunks, func(i, j int) bool {
			switch {
			case chunks[i].sequence != chunks[j].sequence:
				return chunks[i].sequence < chunks[j].sequence
			case chunks[i].start != chunks[j].start:
				return chunks[i].start < chunks[j].start
			default:
				return chunks[i].upload < chunks[j].upload
			}
		})
		if strings.HasPrefix(name, logNames[0]) {
			if start == 0 || (start > 0 && start > chunks[0].start) {
				start = chunks[0].start
			}
			if end < chunks[len(chunks)-1].end {
				end = chunks[len(chunks)-1].end
			}
		}
	}

	// Preserve the order that pail returns the log names to ensure a
	// deterministic merge order.
	chunkGroups := make([]chunkGroup, 0, len(logNames))
	for _, name := range orderedLogNames {
		chunkGroups = append(chunkGroups, chunkGroup{
			name:   name,
			chunks: logChunks[name],
		})
	}

	return chunkGroups, start, end, nil
}

// createChunkKey returns a pail-backed bucket storage key that encodes the
// given log chunk information.
//
// The chunk key is encoded with the chunk info metadata to optimize storage
// and lookup performance.
func (s *logServiceV0) createChunkKey(sequence int, start, end int64, numLines int) string {
	return fmt.Sprintf("%d_%d_%d_%d_%d", sequence, start, end, numLines, time.Now().UnixNano())
}

// parseChunkKey returns the chunk info encoded in the given key.
func (s *logServiceV0) parseChunkKey(prefix, key string) (chunkInfo, error) {
	parsedKey := strings.Split(key, "_")
	if len(parsedKey) < 3 || len(parsedKey) > 5 {
		return chunkInfo{}, errors.New("invalid key format")
	}

	var (
		sequence, idxOffset int
		err                 error
	)
	if len(parsedKey) == 5 {
		sequence, err = strconv.Atoi(parsedKey[0])
		if err != nil {
			return chunkInfo{}, errors.Wrap(err, "parsing sequence")
		}
		idxOffset = 1
	}
	start, err := strconv.ParseInt(parsedKey[idxOffset+0], 10, 64)
	if err != nil {
		return chunkInfo{}, errors.Wrap(err, "parsing start time")
	}
	end, err := strconv.ParseInt(parsedKey[idxOffset+1], 10, 64)
	if err != nil {
		return chunkInfo{}, errors.Wrap(err, "parsing end time")
	}
	numLines, err := strconv.Atoi(parsedKey[idxOffset+2])
	if err != nil {
		return chunkInfo{}, errors.Wrap(err, "parsing num lines")
	}
	var upload int64
	if len(parsedKey) == 4 {
		upload, err = strconv.ParseInt(parsedKey[idxOffset+3], 10, 64)
		if err != nil {
			return chunkInfo{}, errors.Wrap(err, "parsing upload time")
		}
	}

	return chunkInfo{
		key:      prefix + "/" + key,
		sequence: sequence,
		start:    start,
		end:      end,
		numLines: numLines,
		upload:   upload,
	}, nil
}

// formatRawLine formats a log line for storage.
func (s *logServiceV0) formatRawLine(line LogLine) string {
	if line.Data == "" {
		line.Data = "\n"
	} else if line.Data[len(line.Data)-1] != '\n' {
		line.Data += "\n"
	}

	return fmt.Sprintf("%d %d %s", line.Priority, line.Timestamp, line.Data)
}

// getParser returns a function that parses a raw line into the service
// representation of a log line.
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

// GetChunkKeys returns all log chunk keys for the given log names.
func (s *logServiceV0) GetChunkKeys(ctx context.Context, logNames []string) ([]string, error) {
	chunkGroups, _, _, err := s.getLogChunks(ctx, logNames)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, group := range chunkGroups {
		for _, chunk := range group.chunks {
			keys = append(keys, chunk.key)
		}
	}
	return keys, nil
}

// MoveObjectsToBucket moves all objects with the given keys from this log service's bucket to the destination bucket.
func (s *logServiceV0) MoveObjectsToBucket(ctx context.Context, objectKeys []string, destBucket pail.Bucket) error {
	if len(objectKeys) == 0 {
		return nil // nothing to move
	}
	if err := s.bucket.MoveObjects(ctx, destBucket, objectKeys, objectKeys); err != nil {
		return errors.Wrap(err, "bulk moving log chunk objects")
	}
	return nil
}
