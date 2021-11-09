// Package events contains a number of different data types and
// formats that you can use to populate ftdc metrics series.
//
// Custom and CustomPoint
//
// The "custom" types allow you to construct arbirary key-value pairs
// without using maps and have them be well represented in FTDC
// output. Populate and interact with the data sequence as a slice of
// key (string) value (numbers) pairs, which are marshaled in the database
// as an object as a mapping of strings to numbers. The type provides
// some additional helpers for manipulating these data.
package events

import (
	"sort"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/pkg/errors"
)

// CustomPoint represents a computed statistic as a key value
// pair. Use with the Custom type to ensure that the Value types refer
// to number values and ensure consistent round trip semantics through BSON
// and FTDC.
type CustomPoint struct {
	Name  string
	Value interface{}
}

// Custom is a collection of data points designed to store computed
// statistics at an interval. In general you will add a set of
// rolled-up data values to the custom object on an interval and then
// pass that sequence to an ftdc.Collector. Custom implements
// sort.Interface, and the CustomPoint type implements custom bson
// marshalling so that Points are marshaled as an object to facilitate
// their use with the ftdc format.
type Custom []CustomPoint

// MakeCustom creates a Custom slice with the specified size hint.
func MakeCustom(size int) Custom { return make(Custom, 0, size) }

// Add appends a key to the Custom metric. Only accepts go native
// number types and timestamps.
func (ps *Custom) Add(key string, value interface{}) error {
	// TODO: figure out
	switch v := value.(type) {
	case int64, int32, int, bool, time.Time, float64, float32, uint32, uint64:
		*ps = append(*ps, CustomPoint{Name: key, Value: v})
		return nil
	case []int64, []int32, []int, []bool, []time.Time, []float64, []float32, []uint32, []uint64:
		*ps = append(*ps, CustomPoint{Name: key, Value: v})
		return nil
	default:
		return errors.Errorf("type '%T' for key %s is not supported", value, key)
	}
}

// Len is a component of the sort.Interface.
func (ps Custom) Len() int { return len(ps) }

// Less is a component of the sort.Interface.
func (ps Custom) Less(i, j int) bool { return ps[i].Name < ps[j].Name }

// Swap is a component of the sort.Interface.
func (ps Custom) Swap(i, j int) { ps[i], ps[j] = ps[j], ps[i] }

// Sort is a convenience function around a stable sort for the custom
// array.
func (ps Custom) Sort() { sort.Stable(ps) }

func (ps Custom) MarshalBSON() ([]byte, error) { return birch.MarshalDocumentBSON(ps) }

func (ps *Custom) UnmarshalBSON(in []byte) error {
	doc, err := birch.ReadDocument(in)
	if err != nil {
		return errors.Wrap(err, "problem parsing bson document")
	}

	iter := doc.Iterator()
	for iter.Next() {
		elem := iter.Element()
		*ps = append(*ps, CustomPoint{
			Name:  elem.Key(),
			Value: elem.Value().Interface(),
		})
	}

	if err = iter.Err(); err != nil {
		return errors.Wrap(err, "problem reading document")
	}

	return nil
}

func (ps Custom) MarshalDocument() (*birch.Document, error) {
	ps.Sort()

	doc := birch.DC.Make(ps.Len())

	for _, elem := range ps {
		de, err := birch.EC.InterfaceErr(elem.Name, elem.Value)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		doc.Append(de)
	}

	return doc, nil
}
