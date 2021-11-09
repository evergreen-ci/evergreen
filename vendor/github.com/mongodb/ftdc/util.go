package ftdc

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"math"
	"sort"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/birch/bsontype"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

func readDocument(in interface{}) (*birch.Document, error) {
	switch doc := in.(type) {
	case *birch.Document:
		return doc, nil
	case birch.DocumentMarshaler:
		return doc.MarshalDocument()
	case []byte:
		return birch.ReadDocument(doc)
	case birch.Marshaler:
		data, err := doc.MarshalBSON()
		if err != nil {
			return nil, errors.Wrap(err, "problem with unmarshaler")
		}
		return birch.ReadDocument(data)
	case map[string]interface{}, map[string]int, map[string]int64, map[string]uint, map[string]uint64:
		elems := birch.DC.Interface(doc).Elements()
		sort.Stable(elems)
		return birch.DC.Elements(elems...), nil
	case map[string]string:
		return nil, errors.New("cannot use string maps for metrics documents")
	default:
		data, err := bson.Marshal(in)
		if err != nil {
			return nil, errors.Wrap(err, "problem with fallback marshaling")
		}
		return birch.ReadDocument(data)
	}
}

func getOffset(count, sample, metric int) int { return metric*count + sample }

func undeltaFloats(value int64, deltas []int64) []int64 {
	out := make([]int64, 1, len(deltas)+1)
	out[0] = value

	return append(out, deltas...)
}

func undelta(value int64, deltas []int64) []int64 {
	out := make([]int64, len(deltas)+1)
	out[0] = value
	for idx, delta := range deltas {
		out[idx+1] = out[idx] + delta
	}
	return out
}

func encodeSizeValue(val uint32) []byte {
	tmp := make([]byte, 4)

	binary.LittleEndian.PutUint32(tmp, val)

	return tmp
}

func encodeValue(val int64) []byte {
	tmp := make([]byte, binary.MaxVarintLen64)
	num := binary.PutUvarint(tmp, uint64(val))
	return tmp[:num]
}

func compressBuffer(input []byte) ([]byte, error) {
	buf := bytes.NewBuffer([]byte{})
	zbuf := zlib.NewWriter(buf)

	_, err := buf.Write(encodeSizeValue(uint32(len(input))))
	if err != nil {
		return nil, err
	}

	_, err = zbuf.Write(input)
	if err != nil {
		return nil, err
	}

	err = zbuf.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func normalizeFloat(in float64) int64 { return int64(math.Float64bits(in)) }
func restoreFloat(in int64) float64   { return math.Float64frombits(uint64(in)) }
func epochMs(t time.Time) int64       { return t.UnixNano() / 1000000 }
func timeEpocMs(in int64) time.Time   { return time.Unix(in/1000, in%1000*1000000) }

func isNum(num int, val *birch.Value) bool {
	if val == nil {
		return false
	}

	switch val.Type() {
	case bsontype.Int32:
		return val.Int32() == int32(num)
	case bsontype.Int64:
		return val.Int64() == int64(num)
	case bsontype.Double:
		return val.Double() == float64(num)
	default:
		return false
	}
}
