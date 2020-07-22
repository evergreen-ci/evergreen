package ftdc

import (
	"context"
	"io"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/birch/bsontype"
	"github.com/mongodb/ftdc/util"
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

func (c *Chunk) export() (*birch.Document, error) {
	doc := birch.DC.Make(len(c.Metrics))
	sample := 0

	var elem *birch.Element
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
	case bsontype.Int64, bsontype.Timestamp:
		out := make([]int64, len(m.Values))
		copy(out, m.Values)
		return out
	case bsontype.Int32:
		out := make([]int32, len(m.Values))
		for idx, p := range m.Values {
			out[idx] = int32(p)
		}
		return out
	case bsontype.Boolean:
		out := make([]bool, len(m.Values))
		for idx, p := range m.Values {
			out[idx] = p != 0
		}
		return out
	case bsontype.Double:
		out := make([]float64, len(m.Values))
		for idx, p := range m.Values {
			out[idx] = restoreFloat(p)
		}
		return out
	case bsontype.DateTime:
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
	metadata *birch.Document
	document *birch.Document
	pipe     chan *birch.Document
	catcher  util.Catcher
	reflect  bool
}

func (iter *matrixIterator) Close() {
	if iter.chunks != nil {
		iter.chunks.Close()
	}
}

func (iter *matrixIterator) Err() error                { return iter.catcher.Resolve() }
func (iter *matrixIterator) Metadata() *birch.Document { return iter.metadata }
func (iter *matrixIterator) Document() *birch.Document { return iter.document }
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
	var doc *birch.Document
	var err error

	for iter.chunks.Next() {
		chunk := iter.chunks.Chunk()

		if iter.reflect {
			payload, err = bson.Marshal(chunk.exportMatrix())
			if err != nil {
				iter.catcher.Add(err)
				return
			}
			doc, err = birch.ReadDocument(payload)
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
