package ftdc

import (
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/birch/bsontype"
	"github.com/mongodb/ftdc/util"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// Helpers for encoding values from birch documents

type extractedMetrics struct {
	values []*birch.Value
	types  []bsontype.Type
	ts     time.Time
}

func extractMetricsFromDocument(doc *birch.Document) (extractedMetrics, error) {
	metrics := extractedMetrics{}
	iter := doc.Iterator()

	var (
		err  error
		data extractedMetrics
	)

	catcher := util.NewCatcher()

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

func extractMetricsFromArray(array *birch.Array) (extractedMetrics, error) {
	metrics := extractedMetrics{}

	var (
		err  error
		data extractedMetrics
	)

	catcher := util.NewCatcher()
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

func extractMetricsFromValue(val *birch.Value) (extractedMetrics, error) {
	metrics := extractedMetrics{}
	var err error

	btype := val.Type()
	switch btype {
	case bsontype.Array:
		metrics, err = extractMetricsFromArray(val.MutableArray())
		err = errors.WithStack(err)
	case bsontype.EmbeddedDocument:
		metrics, err = extractMetricsFromDocument(val.MutableDocument())
		err = errors.WithStack(err)
	case bsontype.Boolean:
		if val.Boolean() {
			metrics.values = append(metrics.values, birch.VC.Int64(1))
		} else {
			metrics.values = append(metrics.values, birch.VC.Int64(0))
		}
		metrics.types = append(metrics.types, bsontype.Boolean)
	case bsontype.Double:
		metrics.values = append(metrics.values, val)
		metrics.types = append(metrics.types, bsontype.Double)
	case bsontype.Int32:
		metrics.values = append(metrics.values, birch.VC.Int64(int64(val.Int32())))
		metrics.types = append(metrics.types, bsontype.Int32)
	case bsontype.Int64:
		metrics.values = append(metrics.values, val)
		metrics.types = append(metrics.types, bsontype.Int64)
	case bsontype.DateTime:
		metrics.values = append(metrics.values, birch.VC.Int64(epochMs(val.Time())))
		metrics.types = append(metrics.types, bsontype.DateTime)
	case bsontype.Timestamp:
		t, i := val.Timestamp()
		metrics.values = append(metrics.values, birch.VC.Int64(int64(t)), birch.VC.Int64(int64(i)))
		metrics.types = append(metrics.types, bsontype.Timestamp, bsontype.Timestamp)
	}

	return metrics, err
}

func extractDelta(current *birch.Value, previous *birch.Value) (int64, error) {
	switch current.Type() {
	case bsontype.Double:
		return normalizeFloat(current.Double() - previous.Double()), nil
	case bsontype.Int64:
		return current.Int64() - previous.Int64(), nil
	default:
		return 0, errors.Errorf("invalid type %s", current.Type())
	}
}
