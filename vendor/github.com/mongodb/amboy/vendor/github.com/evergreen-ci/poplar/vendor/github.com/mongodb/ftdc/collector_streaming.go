package ftdc

import (
	"io"

	"github.com/pkg/errors"
)

type streamingCollector struct {
	output     io.Writer
	maxSamples int
	count      int
	Collector
}

// NewStreamingCollector wraps the underlying collector, writing the
// data to the underlying writer after the underlying collector is
// filled. This is similar to the batch collector, but allows the
// collector to drop FTDC data from memory. Chunks are flushed to disk
// when the collector as collected the "maxSamples" number of
// samples during the Add operation.
func NewStreamingCollector(maxSamples int, writer io.Writer) Collector {
	return newStreamingCollector(maxSamples, writer)
}

func newStreamingCollector(maxSamples int, writer io.Writer) *streamingCollector {
	return &streamingCollector{
		maxSamples: maxSamples,
		output:     writer,
		Collector: &betterCollector{
			maxDeltas: maxSamples,
		},
	}
}

func (c *streamingCollector) Reset() { c.count = 0; c.Collector.Reset() }
func (c *streamingCollector) Add(in interface{}) error {
	if c.count-1 >= c.maxSamples {
		if err := FlushCollector(c, c.output); err != nil {
			return errors.Wrap(err, "problem flushing collector contents")
		}
	}

	if err := c.Collector.Add(in); err != nil {
		return errors.Wrapf(err, "adding sample #%d", c.count+1)
	}
	c.count++

	return nil
}

// FlushCollector writes the contents of a collector out to an
// io.Writer. This is useful in the context of any collector, but is
// particularly useful in the context of streaming collectors, which
// flush data periodically and may have cached data.
func FlushCollector(c Collector, writer io.Writer) error {
	if writer == nil {
		return errors.New("invalid writer")
	}
	if c.Info().SampleCount == 0 {
		return nil
	}
	payload, err := c.Resolve()
	if err != nil {
		return errors.WithStack(err)
	}

	n, err := writer.Write(payload)
	if err != nil {
		return errors.WithStack(err)
	}
	if n != len(payload) {
		return errors.New("problem flushing data")
	}
	c.Reset()
	return nil
}

type streamingDynamicCollector struct {
	output      io.Writer
	hash        string
	metricCount int
	*streamingCollector
}

// NewStreamingDynamicCollector has the same semantics as the dynamic
// collector but wraps the streaming collector rather than the batch
// collector. Chunks are flushed during the Add() operation when the
// schema changes or the chunk is full.
func NewStreamingDynamicCollector(max int, writer io.Writer) Collector {
	return &streamingDynamicCollector{
		output:             writer,
		streamingCollector: newStreamingCollector(max, writer),
	}
}

func (c *streamingDynamicCollector) Reset() {
	c.streamingCollector = newStreamingCollector(c.streamingCollector.maxSamples, c.output)
	c.metricCount = 0
	c.hash = ""
}

func (c *streamingDynamicCollector) Add(in interface{}) error {
	doc, err := readDocument(in)
	if err != nil {
		return errors.WithStack(err)
	}

	docHash, num := metricKeyHash(doc)
	if c.hash == "" {
		c.hash = docHash
		c.metricCount = num
		if c.streamingCollector.count > 0 {
			if err := FlushCollector(c, c.output); err != nil {
				return errors.WithStack(err)
			}
		}
		return errors.WithStack(c.streamingCollector.Add(doc))
	}

	if c.metricCount != num || c.hash != docHash {
		if err := FlushCollector(c, c.output); err != nil {
			return errors.WithStack(err)
		}
	}

	return errors.WithStack(c.streamingCollector.Add(doc))
}
