package ftdc

import (
	"bytes"
	"io"

	"github.com/evergreen-ci/birch"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// NewUncompressedCollectorJSON constructs a collector that resolves
// data into a stream of JSON documents. The output of these
// uncompressed collectors does not use the FTDC encoding for data,
// and can be read as newline separated JSON.
//
// This collector will not allow you to collect documents with
// different schema (determined by the number of top-level fields.)
//
// If you do not resolve the after receiving the maximum number of
// samples, then additional Add operations will fail.
//
// The metadata for this collector is rendered as the first document
// in the stream.
func NewUncompressedCollectorJSON(maxSamples int) Collector {
	return &uncompressedCollector{
		exportJSON: true,
		batchSize:  maxSamples,
	}
}

// NewUncompressedCollectorBSON constructs a collector that resolves
// data into a stream of BSON documents. The output of these
// uncompressed collectors does not use the FTDC encoding for data,
// and can be read with the bsondump and other related utilites.
//
// This collector will not allow you to collect documents with
// different schema (determined by the number of top-level fields.)
//
// If you do not resolve the after receiving the maximum number of
// samples, then additional Add operations will fail.
//
// The metadata for this collector is rendered as the first document
// in the stream.
func NewUncompressedCollectorBSON(maxSamples int) Collector {
	return &uncompressedCollector{
		exportBSON: true,
		batchSize:  maxSamples,
	}
}

// NewUncompressedCollectorJSON constructs a collector that resolves
// data into a stream of JSON documents. The output of these
// uncompressed collectors does not use the FTDC encoding for data,
// and can be read as newline separated JSON.
//
// This collector will not allow you to collect documents with
// different schema (determined by the number of top-level fields.)
//
// The metadata for this collector is rendered as the first document
// in the stream.
//
// All data is written to the writer when the underlying collector has
// captured its target number of collectors and is automatically
// flushed to the writer during the write operation or when you call
// Close. You can also use the FlushCollector helper.
func NewStreamingUncompressedCollectorJSON(maxSamples int, writer io.Writer) Collector {
	return &streamingCollector{
		maxSamples: maxSamples,
		output:     writer,
		Collector:  NewUncompressedCollectorJSON(maxSamples),
	}
}

// NewUncompressedCollectorBSON constructs a collector that resolves
// data into a stream of BSON documents. The output of these
// uncompressed collectors does not use the FTDC encoding for data,
// and can be read with the bsondump and other related utilites.
//
// This collector will not allow you to collect documents with
// different schema (determined by the number of top-level fields.)
//
// The metadata for this collector is rendered as the first document
// in the stream.
//
// All data is written to the writer when the underlying collector has
// captured its target number of collectors and is automatically
// flushed to the writer during the write operation or when you call
// Close. You can also use the FlushCollector helper.
func NewStreamingUncompressedCollectorBSON(maxSamples int, writer io.Writer) Collector {
	return &streamingCollector{
		maxSamples: maxSamples,
		output:     writer,
		Collector:  NewUncompressedCollectorBSON(maxSamples),
	}
}

// NewStreamingUncompressedCollectorJSON constructs a collector that
// resolves data into a stream of JSON documents. The output of these
// uncompressed collectors does not use the FTDC encoding for data,
// and can be read as newline separated JSON.
//
// The metadata for this collector is rendered as the first document
// in the stream. Additionally, the collector will automatically
// handle schema changes by flushing the previous batch.
//
// All data is written to the writer when the underlying collector has
// captured its target number of collectors and is automatically
// flushed to the writer during the write operation or when you call
// Close. You can also use the FlushCollector helper.
func NewStreamingDynamicUncompressedCollectorBSON(maxSamples int, writer io.Writer) Collector {
	return &streamingDynamicCollector{
		output:             writer,
		streamingCollector: NewStreamingUncompressedCollectorBSON(maxSamples, writer).(*streamingCollector),
	}
}

// NewStreamingUncompressedCollectorBSON constructs a collector that
// resolves data into a stream of BSON documents. The output of these
// uncompressed collectors does not use the FTDC encoding for data,
// and can be read with the bsondump and other related utilites.
//
// The metadata for this collector is rendered as the first document
// in the stream. Additionally, the collector will automatically
// handle schema changes by flushing the previous batch.
//
// All data is written to the writer when the underlying collector has
// captured its target number of collectors and is automatically
// flushed to the writer during the write operation or when you call
// Close. You can also use the FlushCollector helper.
func NewStreamingDynamicUncompressedCollectorJSON(maxSamples int, writer io.Writer) Collector {
	return &streamingDynamicCollector{
		output:             writer,
		streamingCollector: NewStreamingUncompressedCollectorJSON(maxSamples, writer).(*streamingCollector),
	}
}

type uncompressedCollector struct {
	exportBSON  bool
	exportJSON  bool
	batchSize   int
	metricCount int
	metadata    *birch.Document
	samples     []*birch.Document
}

func (c *uncompressedCollector) Reset() {
	c.samples = []*birch.Document{}
	c.metricCount = 0
}

func (c *uncompressedCollector) Info() CollectorInfo {
	return CollectorInfo{
		SampleCount:  len(c.samples),
		MetricsCount: c.metricCount,
	}
}

func (c *uncompressedCollector) SetMetadata(in interface{}) error {
	doc, err := readDocument(in)
	if err != nil {
		return errors.WithStack(err)
	}

	c.metadata = doc
	return nil
}

func (c *uncompressedCollector) Add(in interface{}) error {
	doc, err := readDocument(in)
	if err != nil {
		return errors.WithStack(err)
	}

	if c.metricCount == 0 {
		c.metricCount = doc.Len()
	}

	if doc.Len() != c.metricCount {
		return errors.New("unexpected schema change detected")
	}

	if len(c.samples) >= c.batchSize {
		return errors.New("collector is overfull")
	}

	c.samples = append(c.samples, doc)
	return nil
}

func (c *uncompressedCollector) Resolve() ([]byte, error) {
	if len(c.samples) == 0 {
		return nil, errors.New("no data")
	}

	buf := bytes.NewBuffer([]byte{})

	if c.metadata != nil {
		if err := c.marshalWrite(buf, c.metadata); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	for _, sample := range c.samples {
		if err := c.marshalWrite(buf, sample); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return buf.Bytes(), nil
}

func (c *uncompressedCollector) marshalWrite(buf io.Writer, doc io.WriterTo) error {
	switch {
	case c.exportBSON == c.exportJSON:
		return errors.New("collector export format is not configured")
	case c.exportBSON:
		_, err := doc.WriteTo(buf)
		return errors.WithStack(err)
	case c.exportJSON:
		data, err := bson.MarshalExtJSON(doc, false, false)
		if err != nil {
			return errors.WithStack(err)
		}

		if _, err = buf.Write(data); err != nil {
			return errors.WithStack(err)
		}

		if _, err = buf.Write([]byte("\n")); err != nil {
			return errors.WithStack(err)
		}
		return nil
	default:
		return errors.New("invalid collector configuration")
	}
}
