package ftdc

import (
	"context"
	"io"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/ftdc/util"
)

type Iterator interface {
	Next() bool
	Document() *birch.Document
	Metadata() *birch.Document
	Err() error
	Close()
}

// ReadMetrics returns a standard document iterator that reads FTDC
// chunks. The Documents returned by the iterator are flattened.
func ReadMetrics(ctx context.Context, r io.Reader) Iterator {
	iterctx, cancel := context.WithCancel(ctx)
	iter := &combinedIterator{
		closer:  cancel,
		chunks:  ReadChunks(iterctx, r),
		flatten: true,
		pipe:    make(chan *birch.Document, 100),
		catcher: util.NewCatcher(),
	}

	go iter.worker(iterctx)
	return iter
}

// ReadStructuredMetrics returns a standard document iterator that reads FTDC
// chunks. The Documents returned by the iterator retain the structure
// of the input documents.
func ReadStructuredMetrics(ctx context.Context, r io.Reader) Iterator {
	iterctx, cancel := context.WithCancel(ctx)
	iter := &combinedIterator{
		closer:  cancel,
		chunks:  ReadChunks(iterctx, r),
		flatten: false,
		pipe:    make(chan *birch.Document, 100),
		catcher: util.NewCatcher(),
	}

	go iter.worker(iterctx)
	return iter
}

// ReadMatrix returns a "matrix format" for the data in a chunk. The
// ducments returned by the iterator represent the entire chunk, in
// flattened form, with each field representing a single metric as an
// array of all values for the event.
//
// The matrix documents have full type fidelity, but are not
// substantially less expensive to produce than full iteration.
func ReadMatrix(ctx context.Context, r io.Reader) Iterator {
	iterctx, cancel := context.WithCancel(ctx)
	iter := &matrixIterator{
		closer:  cancel,
		chunks:  ReadChunks(iterctx, r),
		pipe:    make(chan *birch.Document, 25),
		catcher: util.NewCatcher(),
	}

	go iter.worker(iterctx)
	return iter
}

// ReadSeries is similar to the ReadMatrix format, and produces a
// single document per chunk, that contains the flattented keys for
// that chunk, mapped to arrays of all the values of the chunk.
//
// The matrix documents have better type fidelity than raw chunks but
// do not properly collapse the bson timestamp type. To use these
// values produced by the iterator, consider marshaling them directly
// to map[string]interface{} and use a case statement, on the values
// in the map, such as:
//
//     switch v.(type) {
//     case []int32:
//            // ...
//     case []int64:
//            // ...
//     case []bool:
//            // ...
//     case []time.Time:
//            // ...
//     case []float64:
//            // ...
//     }
//
// Although the *birch.Document type does support iteration directly.
func ReadSeries(ctx context.Context, r io.Reader) Iterator {
	iterctx, cancel := context.WithCancel(ctx)
	iter := &matrixIterator{
		closer:  cancel,
		chunks:  ReadChunks(iterctx, r),
		pipe:    make(chan *birch.Document, 25),
		catcher: util.NewCatcher(),
		reflect: true,
	}

	go iter.worker(iterctx)
	return iter
}
