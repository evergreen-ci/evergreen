package log

import (
	"bufio"
	"context"
	"io"

	"github.com/evergreen-ci/pail"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

type chunkInfo struct {
	prefix   string
	key      string
	numLines int
	start    int64
	end      int64
}

type chunkIterator struct {
	opts           chunkIteratorOptions
	idx            int
	lineOffset     int
	lineCount      int
	chunkLineCount int
	next           chan io.ReadCloser
	reader         *chunkReader
	item           LogLine
	catcher        grip.Catcher
	exhausted      bool
	closed         bool
}

type chunkIteratorOptions struct {
	bucket    pail.Bucket
	chunks    []chunkInfo
	parser    lineParser
	start     int64
	end       int64
	lineLimit int
	tailN     int
}

// newChunkIterator returns a LogIterator that iterates over lines of a log
// stored as a set of chunks in pail-backed bucket storage.
func newChunkIterator(ctx context.Context, opts chunkIteratorOptions) *chunkIterator {
	var lineOffset int
	if opts.start > 0 || opts.end > 0 {
		opts.chunks = filterChunksByTimeRange(opts.chunks, opts.start, opts.end)
	}
	if opts.tailN > 0 {
		opts.chunks, lineOffset = filterChunksByTailN(opts.chunks, opts.tailN)
	}
	if opts.lineLimit > 0 {
		opts.chunks = filterChunksByLimit(opts.chunks, opts.lineLimit)
	}

	it := &chunkIterator{
		opts:       opts,
		lineOffset: lineOffset,
		next:       make(chan io.ReadCloser, 1),
		catcher:    grip.NewBasicCatcher(),
	}
	go it.worker(ctx)

	return it
}

func (it *chunkIterator) Next() bool {
	if it.closed || it.exhausted {
		return false
	}
	if it.opts.lineLimit > 0 && it.lineCount == it.opts.lineLimit {
		it.exhausted = true
		return false
	}

	for {
		if it.reader == nil {
			r, ok := <-it.next
			if !ok {
				// If the next channel is closed and there are
				// no errors, this means that the worker go
				// routine successfully exhausted all of the
				// chunks.
				it.exhausted = !it.catcher.HasErrors()
				return false
			}

			it.reader = newChunkReader(r)
			if err := it.skipOffsetLines(); err != nil {
				it.catcher.Add(err)
				return false
			}
		}

		data, err := it.reader.ReadString('\n')
		if err == io.EOF {
			if it.chunkLineCount != it.opts.chunks[it.idx].numLines {
				it.catcher.Add(errors.New("corrupt data"))
				return false
			}
			if err = it.reader.Close(); err != nil {
				it.catcher.Add(err)
				return false
			}

			it.reader = nil
			it.chunkLineCount = 0
			it.idx++
			return it.Next()

		}
		if err != nil {
			it.catcher.Wrap(err, "getting next line")
			return false
		}

		item, err := it.opts.parser(data)
		if err != nil {
			it.catcher.Wrap(err, "parsing log line")
			return false
		}
		it.chunkLineCount++
		it.lineCount++

		if it.opts.end > 0 && item.Timestamp > it.opts.end {
			it.exhausted = true
			return false
		}
		if item.Timestamp >= it.opts.start {
			it.item = item
			break
		}
	}

	return true
}

func (it *chunkIterator) skipOffsetLines() error {
	for it.lineOffset > 0 {
		if _, err := it.reader.ReadString('\n'); err != nil {
			return errors.Wrap(err, "skipping offset lines")
		}
		it.lineOffset--
		it.chunkLineCount++
	}

	return nil
}

func (it *chunkIterator) worker(ctx context.Context) {
	defer func() {
		it.catcher.Add(recovery.HandlePanicWithError(recover(), nil, "log chunk iterator worker"))
		close(it.next)
	}()

	for _, chunk := range it.opts.chunks {
		r, err := it.opts.bucket.Get(ctx, chunk.key)
		if err != nil {
			it.catcher.Wrap(err, "getting chunk from bucket")
			return
		}

		select {
		case it.next <- r:
		case <-ctx.Done():
			it.catcher.Add(ctx.Err())
			return
		}
	}
}

func (it *chunkIterator) Exhausted() bool { return it.exhausted }

func (it *chunkIterator) Err() error { return it.catcher.Resolve() }

func (it *chunkIterator) Item() LogLine { return it.item }

func (it *chunkIterator) Close() error {
	if it.closed {
		return nil
	}
	it.closed = true

	if it.reader != nil {
		return it.reader.Close()
	}
	return nil
}

func filterChunksByTimeRange(chunks []chunkInfo, start, end int64) []chunkInfo {
	var filteredChunks []chunkInfo
	for i := 0; i < len(chunks); i++ {
		if (end > 0 && end < chunks[i].start) || start > chunks[i].end {
			continue
		}
		filteredChunks = append(filteredChunks, chunks[i])
	}

	return filteredChunks
}

func filterChunksByTailN(chunks []chunkInfo, tailN int) ([]chunkInfo, int) {
	var numChunks, lineCount int
	for i := len(chunks) - 1; i >= 0 && lineCount < tailN; i-- {
		lineCount += chunks[i].numLines
		numChunks++
	}

	return chunks[len(chunks)-numChunks:], lineCount - tailN
}

func filterChunksByLimit(chunks []chunkInfo, limit int) []chunkInfo {
	var numChunks, lineCount int
	for i := 0; i < len(chunks) && lineCount < limit; i++ {
		lineCount += chunks[i].numLines
		numChunks++
	}

	return chunks[:numChunks]
}

type chunkReader struct {
	*bufio.Reader
	io.ReadCloser
}

func newChunkReader(r io.ReadCloser) *chunkReader {
	return &chunkReader{
		Reader:     bufio.NewReader(r),
		ReadCloser: r,
	}
}
