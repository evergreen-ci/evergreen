package ftdc

import (
	"math"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/birch/bsontype"
)

////////////////////////////////////////////////////////////////////////
//
// Processores use to return rich (i.e. non-flat) structures from
// metrics slices

func restoreDocument(ref *birch.Document, sample int, metrics []Metric, idx int) (*birch.Document, int) {
	if ref == nil {
		return nil, 0
	}

	iter := ref.Iterator()
	doc := birch.DC.Make(ref.Len())

	var elem *birch.Element

	for iter.Next() {
		refElem := iter.Element()

		elem, idx = restoreElement(refElem, sample, metrics, idx)
		if elem == nil {
			continue
		}
		doc.Append(elem)
	}

	return doc, idx
}

func restoreElement(ref *birch.Element, sample int, metrics []Metric, idx int) (*birch.Element, int) {
	switch ref.Value().Type() {
	case bsontype.ObjectID:
		return nil, idx
	case bsontype.String:
		return nil, idx
	case bsontype.Decimal128:
		return nil, idx
	case bsontype.Array:
		array := ref.Value().MutableArray()

		elems := make([]*birch.Element, 0, array.Len())

		iter := array.Iterator()
		for iter.Next() {
			var item *birch.Element
			// TODO avoid Interface
			item, idx = restoreElement(birch.EC.Interface("", iter.Value()), sample, metrics, idx)
			if item == nil {
				continue
			}

			elems = append(elems, item)
		}

		if iter.Err() != nil {
			return nil, 0
		}

		out := make([]*birch.Value, len(elems))

		for idx := range elems {
			out[idx] = elems[idx].Value()
		}

		return birch.EC.ArrayFromElements(ref.Key(), out...), idx
	case bsontype.EmbeddedDocument:
		var doc *birch.Document

		doc, idx = restoreDocument(ref.Value().MutableDocument(), sample, metrics, idx)
		return birch.EC.SubDocument(ref.Key(), doc), idx
	case bsontype.Boolean:
		value := metrics[idx].Values[sample]
		if value == 0 {
			return birch.EC.Boolean(ref.Key(), false), idx + 1
		}
		return birch.EC.Boolean(ref.Key(), true), idx + 1
	case bsontype.Double:
		return birch.EC.Double(ref.Key(), restoreFloat(metrics[idx].Values[sample])), idx + 1
	case bsontype.Int32:
		return birch.EC.Int32(ref.Key(), int32(metrics[idx].Values[sample])), idx + 1
	case bsontype.Int64:
		return birch.EC.Int64(ref.Key(), metrics[idx].Values[sample]), idx + 1
	case bsontype.DateTime:
		return birch.EC.Time(ref.Key(), timeEpocMs(metrics[idx].Values[sample])), idx + 1
	case bsontype.Timestamp:
		return birch.EC.Timestamp(ref.Key(), uint32(metrics[idx].Values[sample]), uint32(metrics[idx+1].Values[sample])), idx + 2
	default:
		return nil, idx
	}
}

func restoreFlat(t bsontype.Type, key string, value int64) (*birch.Element, bool) {
	switch t {
	case bsontype.Boolean:
		if value == 0 {
			return birch.EC.Boolean(key, false), true
		}
		return birch.EC.Boolean(key, true), true
	case bsontype.Double:
		return birch.EC.Double(key, math.Float64frombits(uint64(value))), true
	case bsontype.Int32:
		return birch.EC.Int32(key, int32(value)), true
	case bsontype.DateTime:
		return birch.EC.Time(key, timeEpocMs(value)), true
	default:
		return birch.EC.Int64(key, value), true
	}
}
