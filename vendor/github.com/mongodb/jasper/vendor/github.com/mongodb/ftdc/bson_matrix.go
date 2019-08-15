package ftdc

import (
	"io"

	"github.com/mongodb/ftdc/bsonx"
	"github.com/pkg/errors"
)

func rehydrateMatrix(metrics []Metric, sample int) (*bsonx.Element, int, error) {
	if sample >= len(metrics) {
		return nil, sample, io.EOF
	}

	// the bsonx library's representation of arrays is more
	// efficent when constructing arrays from documents,
	// otherwise.
	array := bsonx.MakeArray(len(metrics[sample].Values))
	key := metrics[sample].Key()
	switch metrics[sample].originalType {
	case bsonx.TypeBoolean:
		for _, p := range metrics[sample].Values {
			array.AppendInterface(p != 0)
		}
	case bsonx.TypeDouble:
		for _, p := range metrics[sample].Values {
			array.AppendInterface(restoreFloat(p))
		}
	case bsonx.TypeInt64:
		for _, p := range metrics[sample].Values {
			array.AppendInterface(p)
		}
	case bsonx.TypeInt32:
		for _, p := range metrics[sample].Values {
			array.AppendInterface(int32(p))
		}
	case bsonx.TypeDateTime:
		for _, p := range metrics[sample].Values {
			array.AppendInterface(timeEpocMs(p))
		}
	case bsonx.TypeTimestamp:
		for idx, p := range metrics[sample].Values {
			array.AppendInterface(bsonx.Timestamp{T: uint32(p), I: uint32(metrics[sample+1].Values[idx])})
		}
		sample++
	default:
		return nil, sample, errors.New("invalid data type")
	}
	sample++
	return bsonx.EC.Array(key, array), sample, nil
}
