package ftdc

import (
	"time"

	"github.com/mongodb/ftdc/bsonx"
	"github.com/mongodb/ftdc/bsonx/bsontype"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// Helpers for encoding values from bsonx documents

type extractedMetrics struct {
	values []*bsonx.Value
	types  []bsontype.Type
	ts     time.Time
}

func extractMetricsFromDocument(doc *bsonx.Document) (extractedMetrics, error) {
	metrics := extractedMetrics{}
	iter := doc.Iterator()

	var (
		err  error
		data extractedMetrics
	)

	catcher := grip.NewBasicCatcher()

	for iter.Next() {
		data, err = extractMetricsFromValue(iter.Element().Value())
		catcher.Add(err)
		metrics.values = append(metrics.values, data.values...)
		metrics.types = append(metrics.types, data.types...)

		if metrics.ts.IsZero() {
			metrics.ts = data.ts
		}
	}

	catcher.Add(iter.Err())

	if metrics.ts.IsZero() {
		metrics.ts = time.Now()
	}

	return metrics, catcher.Resolve()
}

func extractMetricsFromArray(array *bsonx.Array) (extractedMetrics, error) {
	metrics := extractedMetrics{}

	var (
		err  error
		data extractedMetrics
	)

	catcher := grip.NewBasicCatcher()
	iter := array.Iterator()

	for iter.Next() {
		data, err = extractMetricsFromValue(iter.Value())
		catcher.Add(err)
		metrics.values = append(metrics.values, data.values...)
		metrics.types = append(metrics.types, data.types...)

		if metrics.ts.IsZero() {
			metrics.ts = data.ts
		}
	}

	catcher.Add(iter.Err())

	return metrics, catcher.Resolve()
}

func extractMetricsFromValue(val *bsonx.Value) (extractedMetrics, error) {
	metrics := extractedMetrics{}
	var err error

	btype := val.Type()
	switch btype {
	case bsonx.TypeArray:
		metrics, err = extractMetricsFromArray(val.MutableArray())
		err = errors.WithStack(err)
	case bsonx.TypeEmbeddedDocument:
		metrics, err = extractMetricsFromDocument(val.MutableDocument())
		err = errors.WithStack(err)
	case bsonx.TypeBoolean:
		if val.Boolean() {
			metrics.values = append(metrics.values, bsonx.VC.Int64(1))
		} else {
			metrics.values = append(metrics.values, bsonx.VC.Int64(0))
		}
		metrics.types = append(metrics.types, bsonx.TypeBoolean)
	case bsonx.TypeDouble:
		metrics.values = append(metrics.values, val)
		metrics.types = append(metrics.types, bsonx.TypeDouble)
	case bsonx.TypeInt32:
		metrics.values = append(metrics.values, bsonx.VC.Int64(int64(val.Int32())))
		metrics.types = append(metrics.types, bsonx.TypeInt32)
	case bsonx.TypeInt64:
		metrics.values = append(metrics.values, val)
		metrics.types = append(metrics.types, bsonx.TypeInt64)
	case bsonx.TypeDateTime:
		metrics.values = append(metrics.values, bsonx.VC.Int64(epochMs(val.Time())))
		metrics.types = append(metrics.types, bsonx.TypeDateTime)
	case bsonx.TypeTimestamp:
		t, i := val.Timestamp()
		metrics.values = append(metrics.values, bsonx.VC.Int64(int64(t)), bsonx.VC.Int64(int64(i)))
		metrics.types = append(metrics.types, bsonx.TypeTimestamp, bsonx.TypeTimestamp)
	}

	return metrics, err
}

func extractDelta(current *bsonx.Value, previous *bsonx.Value) (int64, error) {
	switch current.Type() {
	case bsontype.Double:
		return normalizeFloat(current.Double() - previous.Double()), nil
	case bsontype.Int64:
		return current.Int64() - previous.Int64(), nil
	default:
		return 0, errors.Errorf("invalid type %s", current.Type())
	}
}
