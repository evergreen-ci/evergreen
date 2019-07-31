package ftdc

import (
	"context"
	"io"
	"time"

	"github.com/mongodb/ftdc/bsonx"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

func (c *Chunk) exportMatrix() map[string]interface{} {
	out := make(map[string]interface{})
	for _, m := range c.Metrics {
		out[m.Key()] = m.getSeries()
	}
	return out
}

func (c *Chunk) export() (*bsonx.Document, error) {
	doc := bsonx.MakeDocument(len(c.Metrics))
	sample := 0

	var elem *bsonx.Element
	var err error

	for i := 0; i < len(c.Metrics); i++ {
		elem, sample, err = rehydrateMatrix(c.Metrics, sample)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}

		doc.Append(elem)
	}

	return doc, nil
}

func (m *Metric) getSeries() interface{} {
	switch m.originalType {
	case bsonx.TypeInt64, bsonx.TypeTimestamp:
		out := make([]int64, len(m.Values))
		for idx, p := range m.Values {
			out[idx] = p
		}
		return out
	case bsonx.TypeInt32:
		out := make([]int32, len(m.Values))
		for idx, p := range m.Values {
			out[idx] = int32(p)
		}
		return out
	case bsonx.TypeBoolean:
		out := make([]bool, len(m.Values))
		for idx, p := range m.Values {
			out[idx] = p != 0
		}
		return out
	case bsonx.TypeDouble:
		out := make([]float64, len(m.Values))
		for idx, p := range m.Values {
			out[idx] = restoreFloat(p)
		}
		return out
	case bsonx.TypeDateTime:
		out := make([]time.Time, len(m.Values))
		for idx, p := range m.Values {
			out[idx] = timeEpocMs(p)
		}
		return out
	default:
		return nil
	}
}

type matrixIterator struct {
	chunks   *ChunkIterator
	closer   context.CancelFunc
	metadata *bsonx.Document
	document *bsonx.Document
	pipe     chan *bsonx.Document
	catcher  grip.Catcher
	reflect  bool
}

func (iter *matrixIterator) Close() {
	if iter.chunks != nil {
		iter.chunks.Close()
	}
}

func (iter *matrixIterator) Err() error                { return iter.catcher.Resolve() }
func (iter *matrixIterator) Metadata() *bsonx.Document { return iter.metadata }
func (iter *matrixIterator) Document() *bsonx.Document { return iter.document }
func (iter *matrixIterator) Next() bool {
	doc, ok := <-iter.pipe
	if !ok {
		return false
	}

	iter.document = doc
	return true
}

func (iter *matrixIterator) worker(ctx context.Context) {
	defer func() { iter.catcher.Add(iter.chunks.Err()) }()
	defer close(iter.pipe)

	var payload []byte
	var doc *bsonx.Document
	var err error

	for iter.chunks.Next() {
		chunk := iter.chunks.Chunk()

		if iter.reflect {
			payload, err = bson.Marshal(chunk.exportMatrix())
			if err != nil {
				iter.catcher.Add(err)
				return
			}
			doc, err = bsonx.ReadDocument(payload)
			if err != nil {
				iter.catcher.Add(err)
				return
			}
		} else {
			doc, err = chunk.export()
			if err != nil {
				iter.catcher.Add(err)
				return
			}
		}

		select {
		case iter.pipe <- doc:
			continue
		case <-ctx.Done():
			iter.catcher.Add(errors.New("operation aborted"))
			return
		}
	}
}
