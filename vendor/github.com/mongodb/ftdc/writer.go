package ftdc

import (
	"io"

	"github.com/evergreen-ci/birch"
	"github.com/pkg/errors"
)

type writerCollector struct {
	writer    io.WriteCloser
	collector *streamingDynamicCollector
}

func NewWriterCollector(chunkSize int, writer io.WriteCloser) io.WriteCloser {
	return &writerCollector{
		writer: writer,
		collector: &streamingDynamicCollector{
			output:             writer,
			streamingCollector: newStreamingCollector(chunkSize, writer),
		},
	}
}

func (w *writerCollector) Write(in []byte) (int, error) {
	doc, err := birch.ReadDocument(in)
	if err != nil {
		return 0, errors.Wrap(err, "problem reading bson document")
	}
	return len(in), errors.Wrap(w.collector.Add(doc), "problem adding document to collector")
}

func (w *writerCollector) Close() error {
	if err := FlushCollector(w.collector, w.writer); err != nil {
		return errors.Wrap(err, "problem flushing documents to collector")
	}

	return errors.Wrap(w.writer.Close(), "problem closing underlying writer")
}
