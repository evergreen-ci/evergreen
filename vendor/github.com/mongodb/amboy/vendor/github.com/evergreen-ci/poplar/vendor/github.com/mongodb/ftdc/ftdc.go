package ftdc

import (
	"context"
	"strings"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/birch/bsontype"
)

// Chunk represents a 'metric chunk' of data in the FTDC.
type Chunk struct {
	Metrics   []Metric
	nPoints   int
	id        time.Time
	metadata  *birch.Document
	reference *birch.Document
}

func (c *Chunk) GetMetadata() *birch.Document { return c.metadata }
func (c *Chunk) Size() int                    { return c.nPoints }
func (c *Chunk) Len() int                     { return len(c.Metrics) }

// Iterator returns an iterator that you can use to read documents for
// each sample period in the chunk. Documents are returned in collection
// order, with keys flattened and dot-separated fully qualified
// paths.
//
// The documents are constructed from the metrics data lazily.
func (c *Chunk) Iterator(ctx context.Context) Iterator {
	sctx, cancel := context.WithCancel(ctx)
	return &sampleIterator{
		closer:   cancel,
		stream:   c.streamFlattenedDocuments(sctx),
		metadata: c.GetMetadata(),
	}
}

// StructuredIterator returns the contents of the chunk as a sequence
// of documents that (mostly) resemble the original source documents
// (with the non-metrics fields omitted.) The output documents mirror
// the structure of the input documents.
func (c *Chunk) StructuredIterator(ctx context.Context) Iterator {
	sctx, cancel := context.WithCancel(ctx)
	return &sampleIterator{
		closer:   cancel,
		stream:   c.streamDocuments(sctx),
		metadata: c.GetMetadata(),
	}
}

// Metric represents an item in a chunk.
type Metric struct {
	// For metrics that were derived from nested BSON documents,
	// this preserves the path to the field, in support of being
	// able to reconstitute metrics/chunks as a stream of BSON
	// documents.
	ParentPath []string

	// KeyName is the specific field name of a metric in. It is
	// *not* fully qualified with its parent document path, use
	// the Key() method to access a value with more appropriate
	// user facing context.
	KeyName string

	// Values is an array of each value collected for this metric.
	// During decoding, this attribute stores delta-encoded
	// values, but those are expanded during decoding and should
	// never be visible to user.
	Values []int64

	// Used during decoding to expand the delta encoded values. In
	// a properly decoded value, it should always report
	startingValue int64

	originalType bsontype.Type
}

func (m *Metric) Key() string {
	return strings.Join(append(m.ParentPath, m.KeyName), ".")
}
