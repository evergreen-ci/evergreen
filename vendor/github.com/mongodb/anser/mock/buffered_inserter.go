package mock

import "github.com/pkg/errors"

type BufferedInserter struct {
	CloseShouldError  bool
	FlushShouldError  bool
	AppendShouldError bool
	Documents         []interface{}
}

func (bi *BufferedInserter) Append(doc interface{}) error {
	bi.Documents = append(bi.Documents, doc)

	if bi.AppendShouldError {
		return errors.New("append error")
	}

	return nil
}

func (bi *BufferedInserter) Flush() error {
	if bi.FlushShouldError {
		return errors.New("flush error")
	}
	return nil
}

func (bi *BufferedInserter) Close() error {
	if bi.CloseShouldError {
		return errors.New("close error")
	}
	return nil
}
