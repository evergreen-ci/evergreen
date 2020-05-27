package metrics

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/ftdc"
	"github.com/papertrail/go-tail/follower"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// CollectJSONOptions specifies options for a JSON2FTDC collector. You
// must specify EITHER an input Source as a reader or a file
// name.
type CollectJSONOptions struct {
	OutputFilePrefix string
	SampleCount      int
	FlushInterval    time.Duration
	InputSource      io.Reader `json:"-"`
	FileName         string
	Follow           bool
}

func (opts CollectJSONOptions) validate() error {
	bothSpecified := (opts.InputSource == nil && opts.FileName == "")
	neitherSpecifed := (opts.InputSource != nil && opts.FileName != "")

	if bothSpecified || neitherSpecifed {
		return errors.New("must specify exactly one of input source and filename")
	}

	if opts.Follow && opts.FileName == "" {
		return errors.New("follow option must not be specified with a file reader")
	}

	return nil
}

func (opts CollectJSONOptions) getSource() (<-chan *birch.Document, <-chan error) {
	out := make(chan *birch.Document)
	errs := make(chan error, 2)

	switch {
	case opts.InputSource != nil:
		go func() {
			stream := bufio.NewScanner(opts.InputSource)
			defer close(errs)

			for stream.Scan() {
				doc := &birch.Document{}
				err := bson.UnmarshalExtJSON(stream.Bytes(), false, doc)
				if err != nil {
					errs <- err
					return
				}
				out <- doc
			}
		}()
	case opts.FileName != "" && !opts.Follow:
		go func() {
			defer close(errs)
			f, err := os.Open(opts.FileName)
			if err != nil {
				errs <- errors.Wrapf(err, "problem opening data file %s", opts.FileName)
				return
			}
			defer func() { errs <- f.Close() }()
			stream := bufio.NewScanner(f)

			for stream.Scan() {
				doc := &birch.Document{}
				err := bson.UnmarshalExtJSON(stream.Bytes(), false, doc)
				if err != nil {
					errs <- err
					return
				}
				out <- doc
			}
		}()
	case opts.FileName != "" && opts.Follow:
		go func() {
			defer close(errs)

			tail, err := follower.New(opts.FileName, follower.Config{
				Reopen: true,
			})
			if err != nil {
				errs <- errors.Wrapf(err, "problem setting up file follower of '%s'", opts.FileName)
				return
			}
			defer func() {
				tail.Close()
				errs <- tail.Err()
			}()

			for line := range tail.Lines() {
				doc := birch.NewDocument()
				err := bson.UnmarshalExtJSON([]byte(line.String()), false, doc)
				if err != nil {
					errs <- err
					return
				}
				out <- doc
			}
		}()
	default:
		errs <- errors.New("invalid collect options")
		close(errs)
	}
	return out, errs
}

// CollectJSONStream provides a blocking process that reads new-line
// separated JSON documents from a file and creates FTDC data from
// these sources.
//
// The Options structure allows you to define the collection intervals
// and also specify the source. The collector supports reading
// directly from an arbitrary IO reader, or from a file. The "follow"
// option allows you to watch the end of a file for new JSON
// documents, a la "tail -f".
func CollectJSONStream(ctx context.Context, opts CollectJSONOptions) error {
	if err := opts.validate(); err != nil {
		return errors.WithStack(err)
	}

	outputCount := 0
	collector := ftdc.NewDynamicCollector(opts.SampleCount)
	flushTimer := time.NewTimer(opts.FlushInterval)
	defer flushTimer.Stop()

	flusher := func() error {
		fn := fmt.Sprintf("%s.%d", opts.OutputFilePrefix, outputCount)
		info := collector.Info()

		if info.SampleCount == 0 {
			flushTimer.Reset(opts.FlushInterval)
			return nil
		}

		output, err := collector.Resolve()
		if err != nil {
			return errors.Wrap(err, "problem resolving ftdc data")
		}

		if err = ioutil.WriteFile(fn, output, 0600); err != nil {
			return errors.Wrapf(err, "problem writing data to file %s", fn)
		}

		outputCount++
		collector.Reset()
		flushTimer.Reset(opts.FlushInterval)

		return nil
	}

	docs, errs := opts.getSource()

	for {
		select {
		case <-ctx.Done():
			return errors.New("operation aborted")
		case err := <-errs:
			if err == nil || errors.Cause(err) == io.EOF {
				return errors.Wrap(flusher(), "problem flushing results at the end of the file")
			}
			return errors.WithStack(err)
		case doc := <-docs:
			if err := collector.Add(doc); err != nil {
				return errors.Wrap(err, "problem collecting results")
			}
		case <-flushTimer.C:
			return errors.Wrap(flusher(), "problem flushing results at the end of the file")
		}
	}
}
