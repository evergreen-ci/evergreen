package ftdc

import (
	"context"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/ftdc/util"
	"github.com/pkg/errors"
)

type combinedIterator struct {
	closer   context.CancelFunc
	chunks   *ChunkIterator
	sample   *sampleIterator
	metadata *birch.Document
	document *birch.Document
	pipe     chan *birch.Document
	catcher  util.Catcher
	flatten  bool
}

func (iter *combinedIterator) Close() {
	iter.closer()
	if iter.sample != nil {
		iter.sample.Close()
	}

	if iter.chunks != nil {
		iter.chunks.Close()
	}
}

func (iter *combinedIterator) Err() error                { return iter.catcher.Resolve() }
func (iter *combinedIterator) Metadata() *birch.Document { return iter.metadata }
func (iter *combinedIterator) Document() *birch.Document { return iter.document }

func (iter *combinedIterator) Next() bool {
	doc, ok := <-iter.pipe
	if !ok {
		return false
	}

	iter.document = doc
	return true
}

func (iter *combinedIterator) worker(ctx context.Context) {
	defer close(iter.pipe)
	var ok bool

	for iter.chunks.Next() {
		chunk := iter.chunks.Chunk()

		if iter.flatten {
			iter.sample, ok = chunk.Iterator(ctx).(*sampleIterator)
		} else {
			iter.sample, ok = chunk.StructuredIterator(ctx).(*sampleIterator)
		}
		if !ok {
			iter.catcher.Add(errors.New("programmer error"))
			return
		}
		if iter.metadata != nil {
			iter.metadata = chunk.GetMetadata()
		}

		for iter.sample.Next() {
			select {
			case iter.pipe <- iter.sample.Document():
				continue
			case <-ctx.Done():
				iter.catcher.Add(errors.New("operation aborted"))
				return
			}

		}
		iter.catcher.Add(iter.sample.Err())
		iter.sample.Close()
	}
	iter.catcher.Add(iter.chunks.Err())
}
