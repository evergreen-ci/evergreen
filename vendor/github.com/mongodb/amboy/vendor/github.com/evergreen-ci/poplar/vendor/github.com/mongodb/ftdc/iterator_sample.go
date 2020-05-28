package ftdc

import (
	"context"

	"github.com/evergreen-ci/birch"
)

// sampleIterator provides an iterator for iterating through the
// results of a FTDC data chunk as BSON documents.
type sampleIterator struct {
	closer   context.CancelFunc
	stream   <-chan *birch.Document
	sample   *birch.Document
	metadata *birch.Document
}

func (c *Chunk) streamFlattenedDocuments(ctx context.Context) <-chan *birch.Document {
	out := make(chan *birch.Document, 100)

	go func() {
		defer close(out)
		for i := 0; i < c.nPoints; i++ {

			doc := birch.DC.Make(len(c.Metrics))
			for _, m := range c.Metrics {
				elem, ok := restoreFlat(m.originalType, m.Key(), m.Values[i])
				if !ok {
					continue
				}

				doc.Append(elem)
			}

			select {
			case out <- doc:
				continue
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

func (c *Chunk) streamDocuments(ctx context.Context) <-chan *birch.Document {
	out := make(chan *birch.Document, 100)

	go func() {
		defer close(out)

		for i := 0; i < c.nPoints; i++ {
			doc, _ := restoreDocument(c.reference, i, c.Metrics, 0)
			select {
			case <-ctx.Done():
				return
			case out <- doc:
				continue
			}
		}
	}()

	return out
}

// Close releases all resources associated with the iterator.
func (iter *sampleIterator) Close()     { iter.closer() }
func (iter *sampleIterator) Err() error { return nil }

func (iter *sampleIterator) Metadata() *birch.Document { return iter.metadata }

// Document returns the current document in the iterator. It is safe
// to call this method more than once, and the result will only be nil
// before the iterator is advanced.
func (iter *sampleIterator) Document() *birch.Document { return iter.sample }

// Next advances the iterator one document. Returns true when there is
// a document, and false otherwise.
func (iter *sampleIterator) Next() bool {
	doc, ok := <-iter.stream
	if !ok {
		return false
	}

	iter.sample = doc
	return true
}
