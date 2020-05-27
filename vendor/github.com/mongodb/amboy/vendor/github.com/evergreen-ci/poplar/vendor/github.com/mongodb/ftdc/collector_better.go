package ftdc

import (
	"bytes"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/pkg/errors"
)

type betterCollector struct {
	metadata   *birch.Document
	reference  *birch.Document
	startedAt  time.Time
	lastSample *extractedMetrics
	deltas     []int64
	numSamples int
	maxDeltas  int
}

// NewBasicCollector provides a basic FTDC data collector that mirrors
// the server's implementation. The Add method will error if you
// attempt to add more than the specified number of records (plus one,
// as the reference/schema document doesn't count).
func NewBaseCollector(maxSize int) Collector {
	return &betterCollector{
		maxDeltas: maxSize,
	}
}

func (c *betterCollector) SetMetadata(in interface{}) error {
	doc, err := readDocument(in)
	if err != nil {
		return errors.WithStack(err)
	}

	c.metadata = doc
	return nil
}
func (c *betterCollector) Reset() {
	c.reference = nil
	c.lastSample = nil
	c.deltas = nil
	c.numSamples = 0
}

func (c *betterCollector) Info() CollectorInfo {
	var num int

	if c.reference != nil {
		num++
	}

	var metricsCount int
	if c.lastSample != nil {
		metricsCount = len(c.lastSample.values)
	}

	return CollectorInfo{
		SampleCount:  num + c.numSamples,
		MetricsCount: metricsCount,
	}
}

func (c *betterCollector) Add(in interface{}) error {
	doc, err := readDocument(in)
	if err != nil {
		return errors.WithStack(err)
	}

	var metrics extractedMetrics
	if c.reference == nil {
		c.reference = doc
		metrics, err = extractMetricsFromDocument(doc)
		if err != nil {
			return errors.WithStack(err)
		}
		c.startedAt = metrics.ts
		c.lastSample = &metrics
		c.deltas = make([]int64, c.maxDeltas*len(c.lastSample.values))
		return nil
	}

	if c.numSamples >= c.maxDeltas {
		return errors.New("collector is overfull")
	}

	metrics, err = extractMetricsFromDocument(doc)
	if err != nil {
		return errors.WithStack(err)
	}

	if len(metrics.values) != len(c.lastSample.values) {
		return errors.Errorf("unexpected schema change detected for sample %d: [current=%d vs previous=%d]",
			c.numSamples+1, len(metrics.values), len(c.lastSample.values),
		)
	}

	var delta int64
	for idx := range metrics.values {
		if metrics.types[idx] != c.lastSample.types[idx] {
			return errors.Errorf("unexpected schema change detected for sample types: [current=%v vs previous=%v]",
				metrics.types, c.lastSample.types)
		}
		delta, err = extractDelta(metrics.values[idx], c.lastSample.values[idx])
		if err != nil {
			return errors.Wrap(err, "problem parsing data")
		}
		c.deltas[getOffset(c.maxDeltas, c.numSamples, idx)] = delta
	}

	c.numSamples++
	c.lastSample = &metrics

	return nil
}

func (c *betterCollector) Resolve() ([]byte, error) {
	if c.reference == nil {
		return nil, errors.New("no reference document")
	}

	data, err := c.getPayload()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	buf := bytes.NewBuffer([]byte{})
	if c.metadata != nil {
		_, err = birch.NewDocument(
			birch.EC.Time("_id", c.startedAt),
			birch.EC.Int32("type", 0),
			birch.EC.SubDocument("doc", c.metadata)).WriteTo(buf)
		if err != nil {
			return nil, errors.Wrap(err, "problem writing metadata document")
		}
	}

	_, err = birch.NewDocument(
		birch.EC.Time("_id", c.startedAt),
		birch.EC.Int32("type", 1),
		birch.EC.Binary("data", data)).WriteTo(buf)
	if err != nil {
		return nil, errors.Wrap(err, "problem writing metric chunk document")
	}

	return buf.Bytes(), nil
}

func (c *betterCollector) getPayload() ([]byte, error) {
	payload := bytes.NewBuffer([]byte{})
	if _, err := c.reference.WriteTo(payload); err != nil {
		return nil, errors.Wrap(err, "problem writing reference document")
	}

	payload.Write(encodeSizeValue(uint32(len(c.lastSample.values))))
	payload.Write(encodeSizeValue(uint32(c.numSamples)))
	zeroCount := int64(0)
	for i := 0; i < len(c.lastSample.values); i++ {
		for j := 0; j < c.numSamples; j++ {
			delta := c.deltas[getOffset(c.maxDeltas, j, i)]

			if delta == 0 {
				zeroCount++
				continue
			}

			if zeroCount > 0 {
				payload.Write(encodeValue(0))
				payload.Write(encodeValue(zeroCount - 1))
				zeroCount = 0
			}

			payload.Write(encodeValue(delta))
		}
	}
	if zeroCount > 0 {
		payload.Write(encodeValue(0))
		payload.Write(encodeValue(zeroCount - 1))
	}

	data, err := compressBuffer(payload.Bytes())
	if err != nil {
		return nil, errors.Wrap(err, "problem compressing payload")
	}

	return data, nil
}
