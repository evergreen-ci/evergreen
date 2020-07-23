package ftdc

import (
	"io"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/birch/bsontype"
	"github.com/evergreen-ci/birch/types"
	"github.com/pkg/errors"
)

func rehydrateMatrix(metrics []Metric, sample int) (*birch.Element, int, error) {
	if sample >= len(metrics) {
		return nil, sample, io.EOF
	}

	// the birch library's representation of arrays is more
	// efficient when constructing arrays from documents,
	// otherwise.
	array := birch.MakeArray(len(metrics[sample].Values))
	key := metrics[sample].Key()
	switch metrics[sample].originalType {
	case bsontype.Boolean:
		for _, p := range metrics[sample].Values {
			array.AppendInterface(p != 0)
		}
	case bsontype.Double:
		for _, p := range metrics[sample].Values {
			array.AppendInterface(restoreFloat(p))
		}
	case bsontype.Int64:
		for _, p := range metrics[sample].Values {
			array.AppendInterface(p)
		}
	case bsontype.Int32:
		for _, p := range metrics[sample].Values {
			array.AppendInterface(int32(p))
		}
	case bsontype.DateTime:
		for _, p := range metrics[sample].Values {
			array.AppendInterface(timeEpocMs(p))
		}
	case bsontype.Timestamp:
		for idx, p := range metrics[sample].Values {
			array.AppendInterface(types.Timestamp{T: uint32(p), I: uint32(metrics[sample+1].Values[idx])})
		}
		sample++
	default:
		return nil, sample, errors.New("invalid data type")
	}
	sample++
	return birch.EC.Array(key, array), sample, nil
}
