package log

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"time"

	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

type chunkIterator struct {
	opts           chunkIteratorOptions
	lineOffset     int
	lineCount      int
	chunkLineCount int
	next           chan *chunkReader
	done           chan struct{}
	cancelWorker   context.CancelFunc
	reader         *chunkReader
	item           LogLine
	catcher        grip.Catcher
	exhausted      bool
	closed         bool
}

const chunkDownloadTimeout = 60 * time.Second

type chunkIteratorOptions struct {
	bucket          pail.Bucket
	chunks          []chunkInfo
	parser          LineParser
	start           *int64
	end             *int64
	lineLimit       int
	tailN           int
	downloadTimeout time.Duration
}

// newChunkIterator returns a LogIterator that iterates over lines of a log
// stored as a set of chunks in pail-backed bucket storage.
func newChunkIterator(ctx context.Context, opts chunkIteratorOptions) *chunkIterator {
	var lineOffset int
	if opts.start != nil || opts.end != nil {
		opts.chunks = filterChunksByTimeRange(opts.chunks, opts.start, opts.end)
	}
	if opts.tailN > 0 {
		opts.chunks, lineOffset = filterChunksByTailN(opts.chunks, opts.tailN)
	}
	if opts.lineLimit > 0 {
		opts.chunks = filterChunksByLimit(opts.chunks, opts.lineLimit)
	}
	if opts.downloadTimeout == 0 {
		opts.downloadTimeout = chunkDownloadTimeout
	}

	// The worker's context is cancelable so that closing the iterator can
	// abort any chunk download it has in flight.
	ctx, cancel := context.WithCancel(ctx)
	it := &chunkIterator{
		opts:         opts,
		lineOffset:   lineOffset,
		next:         make(chan *chunkReader, 1),
		done:         make(chan struct{}),
		cancelWorker: cancel,
		catcher:      grip.NewBasicCatcher(),
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
			var ok bool
			it.reader, ok = <-it.next
			if !ok {
				// If the next channel is closed and there are
				// no errors, this means that the worker go
				// routine successfully exhausted all of the
				// chunks.
				it.exhausted = !it.catcher.HasErrors()
				return false
			}
		}

		data, err := it.reader.ReadString('\n')
		if err == io.EOF {
			if it.chunkLineCount != it.reader.numLines {
				it.catcher.Add(errors.New("corrupt data"))
				return false
			}
			if err = it.reader.Close(); err != nil {
				it.catcher.Add(err)
				return false
			}

			it.reader = nil
			it.chunkLineCount = 0
			continue

		}
		if err != nil {
			it.catcher.Wrap(err, "getting next line")
			return false
		}
		it.chunkLineCount++

		if it.lineOffset > 0 {
			it.lineOffset--
			continue
		}

		item, err := it.opts.parser(data)
		if err != nil {
			it.catcher.Wrap(err, "parsing log line")
			return false
		}
		it.lineCount++

		if it.opts.end != nil && item.Timestamp > utility.FromInt64Ptr(it.opts.end) {
			it.exhausted = true
			return false
		}
		if item.Timestamp >= utility.FromInt64Ptr(it.opts.start) {
			it.item = item
			break
		}

	}

	return true
}

func (it *chunkIterator) worker(ctx context.Context) {
	defer func() {
		it.catcher.Add(recovery.HandlePanicWithError(recover(), nil, "log chunk iterator worker"))
		close(it.next)
	}()

	for _, chunk := range it.opts.chunks {
		getCtx, cancel := context.WithTimeout(ctx, it.opts.downloadTimeout)
		r, err := it.opts.bucket.Get(getCtx, chunk.key)
		if err != nil {
			cancel()
			if it.isClosed() {
				// Closing the iterator cancels the context, so a download
				// aborted by Close is a clean shutdown rather than an error.
				return
			}
			it.catcher.Wrap(err, "getting chunk from bucket")
			return
		}

		// Read the chunk's data before closing.
		data, err := io.ReadAll(r)
		closeErr := r.Close()
		cancel()
		if err != nil {
			if it.isClosed() {
				return
			}
			it.catcher.Wrap(err, "reading downloaded chunk from bucket")
			return
		}
		if closeErr != nil {
			if it.isClosed() {
				return
			}
			it.catcher.Wrap(closeErr, "closing downloaded chunk from bucket")
			return
		}

		cr := newChunkReader(io.NopCloser(bytes.NewReader(data)), chunk.numLines)

		select {
		case it.next <- cr:
		case <-it.done:
			it.closeUnreadChunk(cr)
			return
		case <-ctx.Done():
			if it.isClosed() {
				it.closeUnreadChunk(cr)
				return
			}
			it.catcher.Add(ctx.Err())
			it.closeUnreadChunk(cr)
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

	close(it.done)
	it.cancelWorker()

	if it.reader != nil {
		return it.reader.Close()
	}
	return nil
}

// isClosed returns whether the consumer has closed the iterator.
func (it *chunkIterator) isClosed() bool {
	select {
	case <-it.done:
		return true
	default:
		return false
	}
}

// closeUnreadChunk releases a fetched chunk that the consumer will never
// read, along with any chunk the worker already buffered ahead of it.
func (it *chunkIterator) closeUnreadChunk(r *chunkReader) {
	_ = r.Close()

	select {
	case buffered := <-it.next:
		_ = buffered.Close()
	default:
	}
}

func filterChunksByTimeRange(chunks []chunkInfo, start, end *int64) []chunkInfo {
	var filteredChunks []chunkInfo
	for i := range chunks {
		if (end != nil && utility.FromInt64Ptr(end) < chunks[i].start) || utility.FromInt64Ptr(start) > chunks[i].end {
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
	numLines int

	*bufio.Reader
	io.ReadCloser
}

func newChunkReader(r io.ReadCloser, numLines int) *chunkReader {
	return &chunkReader{
		numLines:   numLines,
		Reader:     bufio.NewReader(r),
		ReadCloser: r,
	}
}
