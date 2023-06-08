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

type chunkIterator struct {
	bucket               pail.Bucket
	lineParser           func(string) (LogLine, error)
	batchSize            int
	chunks               []chunkInfo
	chunkIndex           int
	start                int64
	end                  int64
	reverse              bool
	lineCount            int
	keyIndex             int
	readers              map[string]io.ReadCloser
	currentReverseReader *reverseLineReader
	currentReader        *bufio.Reader
	currentItem          LogLine
	catcher              grip.Catcher
	exhausted            bool
	closed               bool
}

type chunkInfo struct {
	prefix   string
	key      string
	numLines int
	start    int64
	end      int64
}

// newChunkIterator returns a LogIterator that fetches batches (size set by the
// caller) of chunks from blob storage in parallel while iterating over lines
// of a log.
// TODO: make an options struct?
func newChunkIterator(bucket pail.Bucket, chunks []chunkInfo, batchSize int, start, end int64) LogIterator {
	chunks = filterChunksByTimeRange(chunks, start, end)

	return &chunkIterator{
		bucket:    bucket,
		batchSize: batchSize,
		chunks:    chunks,
		start:     start,
		end:       end,
		catcher:   grip.NewBasicCatcher(),
	}
}

func (i *chunkIterator) Reverse() LogIterator {
	chunks := make([]chunkInfo, len(i.chunks))
	_ = copy(chunks, i.chunks)
	reverseChunks(chunks)

	return &chunkIterator{
		bucket:    i.bucket,
		batchSize: i.batchSize,
		chunks:    chunks,
		start:     i.start,
		end:       i.end,
		reverse:   !i.reverse,
		catcher:   grip.NewBasicCatcher(),
	}
}

func (i *chunkIterator) IsReversed() bool { return i.reverse }

func (i *chunkIterator) Next(ctx context.Context) bool {
	if i.closed {
		return false
	}

	for {
		if i.currentReader == nil && i.currentReverseReader == nil {
			if i.keyIndex >= len(i.chunks) {
				i.exhausted = true
				return false
			}

			reader, ok := i.readers[i.chunks[i.keyIndex].key]
			if !ok {
				if err := i.getNextBatch(ctx); err != nil {
					i.catcher.Add(err)
					return false
				}
				continue
			}

			if i.reverse {
				i.currentReverseReader = newReverseLineReader(reader)
			} else {
				i.currentReader = bufio.NewReader(reader)
			}
		}

		var (
			data string
			err  error
		)
		if i.reverse {
			data, err = i.currentReverseReader.ReadLine()
		} else {
			data, err = i.currentReader.ReadString('\n')
		}
		if err == io.EOF {
			if i.lineCount != i.chunks[i.keyIndex].numLines {
				i.catcher.Add(errors.New("corrupt data"))
			}

			i.currentReverseReader = nil
			i.currentReader = nil
			i.lineCount = 0
			i.keyIndex++

			return i.Next(ctx)
		} else if err != nil {
			i.catcher.Wrap(err, "getting line")
			return false
		}

		item, err := i.lineParser(data)
		if err != nil {
			i.catcher.Wrap(err, "parsing log line")
			return false
		}
		i.lineCount++

		if item.Timestamp > i.end && !i.reverse {
			i.exhausted = true
			return false
		}
		if item.Timestamp < i.start && i.reverse {
			i.exhausted = true
			return false
		}
		if item.Timestamp >= i.start && item.Timestamp <= i.end {
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

	end := i.chunkIndex + i.batchSize
	if end > len(i.chunks) {
		end = len(i.chunks)
	}
	work := make(chan chunkInfo, end-i.chunkIndex)
	for _, chunk := range i.chunks[i.chunkIndex:end] {
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

				r, err := i.bucket.Get(ctx, chunk.key)
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
	filteredChunks := []chunkInfo{}
	for i := 0; i < len(chunks); i++ {
		if (end > 0 && end < chunks[i].start) || start > chunks[i].end {
			continue
		}
		filteredChunks = append(filteredChunks, chunks[i])
	}

	return filteredChunks
}

func reverseChunks(chunks []chunkInfo) {
	for i, j := 0, len(chunks)-1; i < j; i, j = i+1, j-1 {
		chunks[i], chunks[j] = chunks[j], chunks[i]
	}
}

type reverseLineReader struct {
	r     *bufio.Reader
	lines []string
	i     int
}

func newReverseLineReader(r io.Reader) *reverseLineReader {
	return &reverseLineReader{r: bufio.NewReader(r)}
}

func (r *reverseLineReader) ReadLine() (string, error) {
	if r.lines == nil {
		if err := r.getLines(); err != nil {
			return "", errors.Wrap(err, "reading lines")
		}
	}

	r.i--
	if r.i < 0 {
		return "", io.EOF
	}

	return r.lines[r.i], nil
}

func (r *reverseLineReader) getLines() error {
	r.lines = []string{}

	for {
		p, err := r.r.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.WithStack(err)
		}

		r.lines = append(r.lines, p)
	}

	r.i = len(r.lines)

	return nil
}
