package ftdc

import (
	"context"
	"io"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/ftdc/util"
)

// ChunkIterator is a simple iterator for reading off of an FTDC data
// source (e.g. file). The iterator processes chunks batches of
// metrics lazily, reading form the io.Reader every time the iterator
// is advanced.
//
// Use the iterator as follows:
//
//    iter := ReadChunks(ctx, file)
//
//    for iter.Next() {
//        chunk := iter.Chunk()
//
//        // <manipulate chunk>
//
//    }
//
//    if err := iter.Err(); err != nil {
//        return err
//    }
//
// You MUST call the Chunk() method no more than once per iteration.
//
// You shoule check the Err() method when iterator is complete to see
// if there were any issues encountered when decoding chunks.
type ChunkIterator struct {
	pipe    chan *Chunk
	next    *Chunk
	cancel  context.CancelFunc
	closed  bool
	catcher util.Catcher
}

// ReadChunks creates a ChunkIterator from an underlying FTDC data
// source.
func ReadChunks(ctx context.Context, r io.Reader) *ChunkIterator {
	iter := &ChunkIterator{
		catcher: util.NewCatcher(),
		pipe:    make(chan *Chunk, 2),
	}

	ipc := make(chan *birch.Document)
	ctx, iter.cancel = context.WithCancel(ctx)

	go func() {
		iter.catcher.Add(readDiagnostic(ctx, r, ipc))
	}()

	go func() {
		iter.catcher.Add(readChunks(ctx, ipc, iter.pipe))
	}()

	return iter
}

// Next advances the iterator and returns true if the iterator has a
// chunk that is unprocessed. Use the Chunk() method to access the
// iterator.
func (iter *ChunkIterator) Next() bool {
	next, ok := <-iter.pipe
	if !ok {
		return false
	}

	iter.next = next
	return true
}

// Chunk returns a copy of the chunk processed by the iterator. You
// must call Chunk no more than once per iteration. Additional
// accesses to Chunk will panic.
func (iter *ChunkIterator) Chunk() *Chunk {
	return iter.next
}

// Close releases resources of the iterator. Use this method to
// release those resources if you stop iterating before the iterator
// is exhausted. Canceling the context that you used to create the
// iterator has the same effect.
func (iter *ChunkIterator) Close() { iter.cancel(); iter.closed = true }

// Err returns a non-nil error if the iterator encountered any errors
// during iteration.
func (iter *ChunkIterator) Err() error { return iter.catcher.Resolve() }
