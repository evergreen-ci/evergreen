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
	lineCountOffset int
	lineCount       int
	chunkLineCount  int
	keyIndex        int
	readers         map[string]io.ReadCloser
	currentReader   *bufio.Reader
	currentItem     LogLine
	catcher         grip.Catcher
	exhausted       bool
	closed          bool
}

type chunkIteratorOptions struct {
	bucket     pail.Bucket
	chunks     []chunkInfo
	lineParser func(string) (LogLine, error)
	batchSize  int
	start      int64
	end        int64
	lineLimit  int
	tailN      int
}

// newChunkIterator returns a LogIterator that fetches batches (size set by the
// caller) of chunks from blob storage in parallel while iterating over lines
// of a log.
func newChunkIterator(opts chunkIteratorOptions) LogIterator {
	var lineCountOffset int
	opts.chunks = filterChunksByTimeRange(opts.chunks, opts.start, opts.end)
	opts.chunks, lineCountOffset = filterChunksByTailN(opts.chunks, opts.tailN)
	opts.chunks = filterChunksByLimit(opts.chunks, opts.lineLimit)

	return &chunkIterator{
		opts:            opts,
		lineCountOffset: lineCountOffset,
		catcher:         grip.NewBasicCatcher(),
	}
}

func (i *chunkIterator) Next(ctx context.Context) bool {
	if i.closed {
		return false
	}
	if i.opts.lineLimit > 0 && i.lineCount == i.opts.lineLimit {
		return false
	}

	for {
		if i.currentReader == nil {
			if i.keyIndex >= len(i.opts.chunks) {
				i.exhausted = true
				return false
			}

			reader, ok := i.readers[i.opts.chunks[i.keyIndex].key]
			if !ok {
				if err := i.getNextBatch(ctx); err != nil {
					i.catcher.Add(err)
					return false
				}
				continue
			}

			if err := i.skipLines(reader); err != nil {
				i.catcher.Add(err)
				return false
			}
			i.currentReader = bufio.NewReader(reader)
		}

		var (
			data string
			err  error
		)
		data, err = i.currentReader.ReadString('\n')
		if err == io.EOF {
			if i.chunkLineCount != i.opts.chunks[i.keyIndex].numLines {
				i.catcher.Add(errors.New("corrupt data"))
			}

			i.currentReader = nil
			i.lineCount = 0
			i.keyIndex++
			return i.Next(ctx)
		} else if err != nil {
			i.catcher.Wrap(err, "getting line")
			return false
		}

		item, err := i.opts.lineParser(data)
		if err != nil {
			i.catcher.Wrap(err, "parsing log line")
			return false
		}
		i.chunkLineCount++
		i.lineCount++

		if item.Timestamp > i.opts.end {
			i.exhausted = true
			return false
		}
		if item.Timestamp >= i.opts.start && item.Timestamp <= i.opts.end {
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

func (i *chunkIterator) skipLines(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for i.lineCountOffset > 0 && scanner.Scan() {
		i.lineCountOffset--
		i.chunkLineCount++
	}

	return errors.Wrap(scanner.Err(), "skipping offset lines")
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
	var (
		filteredChunks []chunkInfo
		lineCount      int
	)
	for i := len(chunks) - 1; i >= 0 && lineCount < tailN; i-- {
		filteredChunks = append(filteredChunks, chunks[i])
		lineCount += chunks[i].numLines
	}

	return filteredChunks, lineCount - tailN
}

func filterChunksByLimit(chunks []chunkInfo, limit int) []chunkInfo {
	var (
		filteredChunks []chunkInfo
		lineCount      int
	)
	for i := 0; i < len(chunks) && lineCount < limit; i++ {
		filteredChunks = append(filteredChunks, chunks[i])
		lineCount += chunks[i].numLines
	}

	return filteredChunks
}
