package log

import (
	"bufio"
	"context"
	"io"
	"runtime"
	"sync"

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
	opts            chunkIteratorOptions
	chunkIndex      int
	keyIndex        int
	lineCountOffset int
	lineCount       int
	chunkLineCount  int
	readers         map[string]io.ReadCloser
	reader          *bufio.Reader
	currentItem     LogLine
	catcher         grip.Catcher
	exhausted       bool
	closed          bool
}

type chunkIteratorOptions struct {
	bucket    pail.Bucket
	chunks    []chunkInfo
	parser    lineParser
	batchSize int
	start     int64
	end       int64
	lineLimit int
	tailN     int
}

// newChunkIterator returns a LogIterator that fetches batches (size set by the
// caller) of chunks from blob storage in parallel while iterating over lines
// of a log.
func newChunkIterator(opts chunkIteratorOptions) *chunkIterator {
	var lineCountOffset int
	if opts.start > 0 || opts.end > 0 {
		opts.chunks = filterChunksByTimeRange(opts.chunks, opts.start, opts.end)
	}
	if opts.tailN > 0 {
		opts.chunks, lineCountOffset = filterChunksByTailN(opts.chunks, opts.tailN)
	}
	if opts.lineLimit > 0 {
		opts.chunks = filterChunksByLimit(opts.chunks, opts.lineLimit)
	}

	return &chunkIterator{
		opts:            opts,
		lineCountOffset: lineCountOffset,
		catcher:         grip.NewBasicCatcher(),
	}
}

func (i *chunkIterator) Next(ctx context.Context) bool {
	if i.closed || i.exhausted {
		return false
	}
	if i.opts.lineLimit > 0 && i.lineCount == i.opts.lineLimit {
		i.exhausted = true
		return false
	}

	for {
		if i.reader == nil {
			if i.keyIndex >= len(i.opts.chunks) {
				i.exhausted = true
				return false
			}

			r, ok := i.readers[i.opts.chunks[i.keyIndex].key]
			if !ok {
				if err := i.getNextBatch(ctx); err != nil {
					i.catcher.Add(err)
					return false
				}
				continue
			}

			i.reader = bufio.NewReader(r)
			if err := i.skipLines(); err != nil {
				i.catcher.Add(err)
				return false
			}
		}

		data, err := i.reader.ReadString('\n')
		if err == io.EOF {
			if i.chunkLineCount != i.opts.chunks[i.keyIndex].numLines {
				i.catcher.Add(errors.New("corrupt data"))
			}

			i.reader = nil
			i.chunkLineCount = 0
			i.keyIndex++
			return i.Next(ctx)

		}
		if err != nil {
			i.catcher.Wrap(err, "getting next line")
			return false
		}

		item, err := i.opts.parser(data)
		if err != nil {
			i.catcher.Wrap(err, "parsing log line")
			return false
		}
		i.chunkLineCount++
		i.lineCount++

		if i.opts.end > 0 && item.Timestamp > i.opts.end {
			i.exhausted = true
			return false
		}
		if item.Timestamp >= i.opts.start {
			i.currentItem = item
			break
		}
	}

	return true
}

func (i *chunkIterator) getNextBatch(ctx context.Context) error {
	catcher := grip.NewBasicCatcher()
	for _, r := range i.readers {
		catcher.Add(r.Close())
	}
	if err := catcher.Resolve(); err != nil {
		return errors.Wrap(err, "closing readers")
	}

	end := i.chunkIndex + i.opts.batchSize
	if end > len(i.opts.chunks) {
		end = len(i.opts.chunks)
	}
	work := make(chan chunkInfo, end-i.chunkIndex)
	for _, chunk := range i.opts.chunks[i.chunkIndex:end] {
		work <- chunk
	}
	close(work)

	var (
		wg  sync.WaitGroup
		mux sync.Mutex
	)
	readers := map[string]io.ReadCloser{}
	catcher = grip.NewBasicCatcher()
	for j := 0; j < runtime.NumCPU(); j++ {
		wg.Add(1)
		go func() {
			defer func() {
				catcher.Add(recovery.HandlePanicWithError(recover(), nil, "log iterator worker"))
				wg.Done()
			}()

			for chunk := range work {
				if err := ctx.Err(); err != nil {
					catcher.Add(err)
					return
				}

				r, err := i.opts.bucket.Get(ctx, chunk.key)
				if err != nil {
					catcher.Add(err)
					return
				}
				mux.Lock()
				readers[chunk.key] = r
				mux.Unlock()
			}
		}()
	}
	wg.Wait()

	i.chunkIndex = end
	i.readers = readers
	return errors.Wrap(catcher.Resolve(), "downloading log artifacts")
}

func (i *chunkIterator) skipLines() error {
	for i.lineCountOffset > 0 {
		if _, err := i.reader.ReadString('\n'); err != nil {
			return errors.Wrap(err, "skipping offset lines")
		}
		i.lineCountOffset--
		i.chunkLineCount++
	}

	return nil
}

func (i *chunkIterator) Exhausted() bool { return i.exhausted }

func (i *chunkIterator) Err() error { return i.catcher.Resolve() }

func (i *chunkIterator) Item() LogLine { return i.currentItem }

func (i *chunkIterator) Close() error {
	i.closed = true
	catcher := grip.NewBasicCatcher()

	for _, r := range i.readers {
		catcher.Add(r.Close())
	}

	return catcher.Resolve()
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
