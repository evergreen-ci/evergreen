package testutil

import (
	"bytes"
	"fmt"
	"math/rand"
	"strconv"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/birch/bsontype"
	"github.com/pkg/errors"
)

func CreateEventRecord(count, duration, size, workers int64) *birch.Document {
	return birch.NewDocument(
		birch.EC.Int64("count", count),
		birch.EC.Int64("duration", duration),
		birch.EC.Int64("size", size),
		birch.EC.Int64("workers", workers),
	)
}

func RandFlatDocument(numKeys int) *birch.Document {
	doc := birch.NewDocument()
	for i := 0; i < numKeys; i++ {
		doc.Append(birch.EC.Int64(fmt.Sprint(i), rand.Int63n(int64(numKeys)*1)))
	}

	return doc
}

func RandFlatDocumentWithFloats(numKeys int) *birch.Document {
	doc := birch.NewDocument()
	for i := 0; i < numKeys; i++ {
		doc.Append(birch.EC.Double(fmt.Sprintf("%d_float", i), rand.Float64()))
		doc.Append(birch.EC.Int64(fmt.Sprintf("%d_long", i), rand.Int63()))
	}
	return doc
}

func RandComplexDocument(numKeys, otherNum int) *birch.Document {
	doc := birch.NewDocument()

	for i := 0; i < numKeys; i++ {
		doc.Append(birch.EC.Int64(fmt.Sprintln(numKeys, otherNum), rand.Int63n(int64(numKeys)*1)))
		doc.Append(birch.EC.Double(fmt.Sprintln("float", numKeys, otherNum), rand.Float64()))

		if otherNum%5 == 0 {
			ar := birch.NewArray()
			for ii := int64(0); i < otherNum; i++ {
				ar.Append(birch.VC.Int64(rand.Int63n(1 + ii*int64(numKeys))))
			}
			doc.Append(birch.EC.Array(fmt.Sprintln("first", numKeys, otherNum), ar))
		}

		if otherNum%3 == 0 {
			doc.Append(birch.EC.SubDocument(fmt.Sprintln("second", numKeys, otherNum), RandFlatDocument(otherNum)))
		}

		if otherNum%12 == 0 {
			doc.Append(birch.EC.SubDocument(fmt.Sprintln("third", numKeys, otherNum), RandComplexDocument(otherNum, 10)))
		}
	}

	return doc
}

func IsMetricsDocument(key string, doc *birch.Document) ([]string, int) {
	iter := doc.Iterator()
	keys := []string{}
	seen := 0
	for iter.Next() {
		elem := iter.Element()
		k, num := IsMetricsValue(fmt.Sprintf("%s/%s", key, elem.Key()), elem.Value())
		if num > 0 {
			seen += num
			keys = append(keys, k...)
		}
	}

	return keys, seen
}

func IsMetricsArray(key string, array *birch.Array) ([]string, int) {
	idx := 0
	numKeys := 0
	keys := []string{}
	iter := array.Iterator()
	for iter.Next() {
		ks, num := IsMetricsValue(key+strconv.Itoa(idx), iter.Value())

		if num > 0 {
			numKeys += num
			keys = append(keys, ks...)
		}

		idx++
	}

	return keys, numKeys
}

func IsMetricsValue(key string, val *birch.Value) ([]string, int) {
	switch val.Type() {
	case bsontype.ObjectID:
		return nil, 0
	case bsontype.String:
		return nil, 0
	case bsontype.Decimal128:
		return nil, 0
	case bsontype.Array:
		return IsMetricsArray(key, val.MutableArray())
	case bsontype.EmbeddedDocument:
		return IsMetricsDocument(key, val.MutableDocument())
	case bsontype.Boolean:
		return []string{key}, 1
	case bsontype.Double:
		return []string{key}, 1
	case bsontype.Int32:
		return []string{key}, 1
	case bsontype.Int64:
		return []string{key}, 1
	case bsontype.DateTime:
		return []string{key}, 1
	case bsontype.Timestamp:
		return []string{key}, 2
	default:
		return nil, 0
	}
}

type NoopWRiter struct {
	bytes.Buffer
}

func (n *NoopWRiter) Write(in []byte) (int, error) { return n.Buffer.Write(in) }
func (n *NoopWRiter) Close() error                 { return nil }

type ErrorWriter struct {
	bytes.Buffer
}

func (n *ErrorWriter) Write(in []byte) (int, error) { return 0, errors.New("foo") }
func (n *ErrorWriter) Close() error                 { return errors.New("close") }

type marshaler struct {
	doc *birch.Document
}

func (m *marshaler) MarshalBSON() ([]byte, error) {
	if m.doc == nil {
		return nil, errors.New("empty")
	}
	return m.doc.MarshalBSON()
}
