package ftdc

import (
	"fmt"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/birch/bsontype"
)

////////////////////////////////////////////////////////////////////////
//
// Helpers for parsing the timeseries data from a metrics payload

func metricForDocument(path []string, d *birch.Document) []Metric {
	iter := d.Iterator()
	o := []Metric{}

	for iter.Next() {
		e := iter.Element()

		o = append(o, metricForType(e.Key(), path, e.Value())...)
	}

	return o
}

func metricForArray(key string, path []string, a *birch.Array) []Metric {
	if a == nil {
		return []Metric{}
	}

	iter := a.Iterator() // ignore the error which can never be non-nil
	o := []Metric{}
	idx := 0
	for iter.Next() {
		o = append(o, metricForType(fmt.Sprintf("%s.%d", key, idx), path, iter.Value())...)
		idx++
	}

	return o
}

func metricForType(key string, path []string, val *birch.Value) []Metric {
	switch val.Type() {
	case bsontype.ObjectID:
		return []Metric{}
	case bsontype.String:
		return []Metric{}
	case bsontype.Decimal128:
		return []Metric{}
	case bsontype.Array:
		return metricForArray(key, path, val.MutableArray())
	case bsontype.EmbeddedDocument:
		path = append(path, key)

		o := []Metric{}
		for _, ne := range metricForDocument(path, val.MutableDocument()) {
			o = append(o, Metric{
				ParentPath:    path,
				KeyName:       ne.KeyName,
				startingValue: ne.startingValue,
				originalType:  ne.originalType,
			})
		}
		return o
	case bsontype.Boolean:
		if val.Boolean() {
			return []Metric{
				{
					ParentPath:    path,
					KeyName:       key,
					startingValue: 1,
					originalType:  val.Type(),
				},
			}
		}
		return []Metric{
			{
				ParentPath:    path,
				KeyName:       key,
				startingValue: 0,
				originalType:  val.Type(),
			},
		}
	case bsontype.Double:
		return []Metric{
			{
				ParentPath:    path,
				KeyName:       key,
				startingValue: normalizeFloat(val.Double()),
				originalType:  val.Type(),
			},
		}
	case bsontype.Int32:
		return []Metric{
			{
				ParentPath:    path,
				KeyName:       key,
				startingValue: int64(val.Int32()),
				originalType:  val.Type(),
			},
		}
	case bsontype.Int64:
		return []Metric{
			{
				ParentPath:    path,
				KeyName:       key,
				startingValue: val.Int64(),
				originalType:  val.Type(),
			},
		}
	case bsontype.DateTime:
		return []Metric{
			{
				ParentPath:    path,
				KeyName:       key,
				startingValue: epochMs(val.Time()),
				originalType:  val.Type(),
			},
		}
	case bsontype.Timestamp:
		t, i := val.Timestamp()
		return []Metric{
			{
				ParentPath:    path,
				KeyName:       key,
				startingValue: int64(t) * 1000,
				originalType:  val.Type(),
			},
			{
				ParentPath:    path,
				KeyName:       key + ".inc",
				startingValue: int64(i),
				originalType:  val.Type(),
			},
		}
	default:
		return []Metric{}
	}
}
