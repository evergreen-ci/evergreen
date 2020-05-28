package ftdc

import (
	"fmt"
	"hash/fnv"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/birch/bsontype"
	"github.com/evergreen-ci/birch/decimal"
	"github.com/evergreen-ci/birch/types"
	"github.com/mongodb/ftdc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestFlattenArray(t *testing.T) {
	t.Run("NilArray", func(t *testing.T) {
		out := metricForArray("", nil, nil)
		assert.NotNil(t, out)
		assert.Len(t, out, 0)
	})
	t.Run("EmptyArray", func(t *testing.T) {
		out := metricForArray("", nil, birch.NewArray())
		assert.NotNil(t, out)
		assert.Len(t, out, 0)
	})
	t.Run("TwoElements", func(t *testing.T) {
		m := metricForArray("foo", nil, birch.NewArray(birch.VC.Boolean(true), birch.VC.Boolean(false)))
		assert.NotNil(t, m)
		assert.Len(t, m, 2)

		assert.Equal(t, m[0].Key(), "foo.0")
		assert.Equal(t, m[1].Key(), "foo.1")
		assert.Equal(t, int64(1), m[0].startingValue)
		assert.Equal(t, int64(0), m[1].startingValue)
	})
	t.Run("TwoElementsWithSkippedValue", func(t *testing.T) {
		m := metricForArray("foo", nil, birch.NewArray(birch.VC.String("foo"), birch.VC.Boolean(false)))
		assert.NotNil(t, m)
		assert.Len(t, m, 1)

		assert.Equal(t, m[0].Key(), "foo.1")
		assert.Equal(t, int64(0), m[0].startingValue)
	})
	t.Run("ArrayWithOnlyStrings", func(t *testing.T) {
		out := metricForArray("foo", nil, birch.NewArray(birch.VC.String("foo"), birch.VC.String("bar")))
		assert.NotNil(t, out)
		assert.Len(t, out, 0)
	})
}

func TestReadDocument(t *testing.T) {
	for _, test := range []struct {
		name        string
		in          interface{}
		shouldError bool
		len         int
	}{
		{
			name:        "EmptyBytes",
			in:          []byte{},
			shouldError: true,
			len:         0,
		},
		{
			name:        "Nil",
			in:          nil,
			shouldError: true,
			len:         0,
		},
		{
			name: "NewDocument",
			in:   birch.NewDocument(),
			len:  0,
		},
		{
			name:        "NewReader",
			in:          birch.Reader{},
			shouldError: true,
			len:         0,
		},
		{
			name: "EmptyStruct",
			in:   struct{}{},
			len:  0,
		},
		{
			name: "DocumentOneValue",
			in:   birch.NewDocument(birch.EC.ObjectID("_id", types.NewObjectID())),
			len:  1,
		},
		{
			name: "StructWithValuesAndTags",
			in: struct {
				Name    string    `bson:"name"`
				Time    time.Time `bson:"time"`
				Counter int64     `bson:"counter"`
			}{
				Name:    "foo",
				Time:    time.Now(),
				Counter: 42,
			},
			len: 3,
		},
		{
			name: "StructWithValues",
			in: struct {
				Name    string
				Time    time.Time
				Counter int64
			}{
				Name:    "foo",
				Time:    time.Now(),
				Counter: 42,
			},
			len: 3,
		},
		{
			name: "Reader",
			in: func() birch.Reader {
				out, err := birch.NewDocument(
					birch.EC.String("foo", "bar"),
					birch.EC.Int64("baz", 33)).MarshalBSON()
				require.NoError(t, err)
				return birch.Reader(out)
			}(),
			len: 2,
		},
		{
			name: "Raw",
			in: func() bson.Raw {
				out, err := birch.NewDocument(
					birch.EC.String("foo", "bar"),
					birch.EC.Boolean("wat", false),
					birch.EC.Time("ts", time.Now()),
					birch.EC.Int64("baz", 33)).MarshalBSON()
				require.NoError(t, err)
				return bson.Raw(out)
			}(),
			len: 4,
		},
		{
			name:        "MarshalerError",
			in:          &marshaler{},
			shouldError: true,
		},
		{
			name: "MarshalerEmtpy",
			in: &marshaler{
				birch.NewDocument(),
			},
		},
		{
			name: "MarshalerValue",
			in: &marshaler{
				birch.NewDocument(birch.EC.String("foo", "bat")),
			},
			len: 1,
		},
		{
			name:        "Map",
			in:          map[string]interface{}{},
			shouldError: false,
		},
		{
			name:        "MapPopulated",
			in:          map[string]interface{}{"foo": "bar"},
			shouldError: false,
			len:         1,
		},
		{
			name:        "StringMapEmpty",
			in:          map[string]string{},
			shouldError: true,
		},
		{
			name:        "StringMap",
			in:          map[string]string{"foo": "bar"},
			shouldError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc, err := readDocument(test.in)
			if test.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if doc != nil {
				assert.Equal(t, test.len, doc.Len())
			}
		})
	}

}

func TestBSONValueToMetric(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		Name  string
		Value *birch.Value
		Key   string
		Path  []string

		Expected  int64
		OutputLen int
	}{
		{
			Name:  "ObjectID",
			Value: birch.VC.ObjectID(types.NewObjectID()),
		},
		{
			Name:  "StringShort",
			Value: birch.VC.String("Hello World"),
		},
		{
			Name:  "StringEmpty",
			Value: birch.VC.String(""),
		},
		{
			Name:  "StringLooksLikeNumber",
			Value: birch.VC.String("42"),
		},
		{
			Name:  "Decimal128Empty",
			Value: birch.VC.Decimal128(decimal.Decimal128{}),
		},
		{
			Name:  "Decimal128",
			Value: birch.VC.Decimal128(decimal.NewDecimal128(33, 43)),
		},
		{
			Name:  "DBPointer",
			Value: birch.VC.DBPointer("foo.bar", types.NewObjectID()),
		},
		{
			Name:      "BoolTrue",
			Path:      []string{"one", "two"},
			Key:       "foo",
			Value:     birch.VC.Boolean(true),
			OutputLen: 1,
			Expected:  1,
		},
		{
			Name:      "BoolFalse",
			Key:       "foo",
			Path:      []string{"one", "two"},
			Value:     birch.VC.Boolean(false),
			OutputLen: 1,
			Expected:  0,
		},
		{
			Name:  "ArrayEmpty",
			Key:   "foo",
			Path:  []string{"one", "two"},
			Value: birch.VC.ArrayFromValues(),
		},
		{
			Name:  "ArrayOfStrings",
			Key:   "foo",
			Path:  []string{"one", "two"},
			Value: birch.VC.ArrayFromValues(birch.VC.String("one"), birch.VC.String("two")),
		},
		{
			Name:      "ArrayOfMixed",
			Key:       "foo",
			Path:      []string{"one", "two"},
			Value:     birch.VC.ArrayFromValues(birch.VC.String("one"), birch.VC.Boolean(true)),
			OutputLen: 1,
			Expected:  1,
		},
		{
			Name:      "ArrayOfBools",
			Key:       "foo",
			Path:      []string{"one", "two"},
			Value:     birch.VC.ArrayFromValues(birch.VC.Boolean(true), birch.VC.Boolean(true)),
			OutputLen: 2,
			Expected:  1,
		},
		{
			Name:  "EmptyDocument",
			Value: birch.VC.DocumentFromElements(),
		},
		{
			Name:  "DocumentWithNonMetricFields",
			Value: birch.VC.DocumentFromElements(birch.EC.String("foo", "bar")),
		},
		{
			Name:      "DocumentWithOneValue",
			Value:     birch.VC.DocumentFromElements(birch.EC.Boolean("foo", true)),
			Key:       "foo",
			Path:      []string{"exists"},
			OutputLen: 1,
			Expected:  1,
		},
		{
			Name:      "Double",
			Value:     birch.VC.Double(42.42),
			OutputLen: 1,
			Expected:  normalizeFloat(42.42),
			Key:       "foo",
			Path:      []string{"really", "exists"},
		},
		{
			Name:      "OtherDouble",
			Value:     birch.VC.Double(42.0),
			OutputLen: 1,
			Expected:  normalizeFloat(42.0),
			Key:       "foo",
			Path:      []string{"really", "exists"},
		},
		{
			Name:      "DoubleZero",
			Value:     birch.VC.Double(0),
			OutputLen: 1,
			Expected:  0,
			Key:       "foo",
			Path:      []string{"really", "exists"},
		},
		{
			Name:      "DoubleShortZero",
			Value:     birch.VC.Int32(0),
			OutputLen: 1,
			Expected:  0,
			Key:       "foo",
			Path:      []string{"really", "exists"},
		},
		{
			Name:      "DoubleShort",
			Value:     birch.VC.Int32(42),
			OutputLen: 1,
			Expected:  42,
			Key:       "foo",
			Path:      []string{"really", "exists"},
		},
		{
			Name:      "DoubleLong",
			Value:     birch.VC.Int64(42),
			OutputLen: 1,
			Expected:  42,
			Key:       "foo",
			Path:      []string{"really", "exists"},
		},
		{
			Name:      "DoubleLongZero",
			Value:     birch.VC.Int64(0),
			OutputLen: 1,
			Expected:  0,
			Key:       "foo",
			Path:      []string{"really", "exists"},
		},
		{
			Name:      "DatetimeZero",
			Value:     birch.VC.DateTime(0),
			OutputLen: 1,
			Expected:  0,
			Key:       "foo",
			Path:      []string{"really", "exists"},
		},
		{
			Name:      "DatetimeLarge",
			Value:     birch.EC.Time("", now).Value(),
			OutputLen: 1,
			Expected:  epochMs(now),
			Key:       "foo",
			Path:      []string{"really", "exists"},
		},
		{
			Name:      "TimeStamp",
			Value:     birch.VC.Timestamp(100, 100),
			OutputLen: 2,
			Expected:  100000,
			Key:       "foo",
			Path:      []string{"really", "exists"},
		},
	} {
		t.Run(test.Name, func(t *testing.T) {
			m := metricForType(test.Key, test.Path, test.Value)
			assert.Len(t, m, test.OutputLen)

			if test.OutputLen > 0 {
				assert.Equal(t, test.Expected, m[0].startingValue)
				assert.True(t, strings.HasPrefix(m[0].KeyName, test.Key))
				assert.True(t, strings.HasPrefix(m[0].Key(), strings.Join(test.Path, ".")))
			} else {
				assert.NotNil(t, m)
			}
		})
	}
}

func TestExtractingMetrics(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		Name              string
		Value             *birch.Value
		ExpectedCount     int
		FirstEncodedValue int64
		NumEncodedValues  int
		Types             []bsontype.Type
	}{
		{
			Name:              "IgnoredType",
			Value:             birch.VC.Null(),
			ExpectedCount:     0,
			FirstEncodedValue: 0,
			NumEncodedValues:  0,
		},
		{
			Name:              "ObjectID",
			Value:             birch.VC.ObjectID(types.NewObjectID()),
			ExpectedCount:     0,
			FirstEncodedValue: 0,
			NumEncodedValues:  0,
		},
		{
			Name:              "String",
			Value:             birch.VC.String("foo"),
			ExpectedCount:     0,
			FirstEncodedValue: 0,
			NumEncodedValues:  0,
		},
		{
			Name:              "Decimal128",
			Value:             birch.VC.Decimal128(decimal.NewDecimal128(42, 42)),
			ExpectedCount:     0,
			FirstEncodedValue: 0,
			NumEncodedValues:  0,
		},
		{
			Name:              "BoolTrue",
			Value:             birch.VC.Boolean(true),
			ExpectedCount:     1,
			FirstEncodedValue: 1,
			NumEncodedValues:  1,
			Types:             []bsontype.Type{bsontype.Boolean},
		},
		{
			Name:              "BoolFalse",
			Value:             birch.VC.Boolean(false),
			ExpectedCount:     1,
			FirstEncodedValue: 0,
			NumEncodedValues:  1,
			Types:             []bsontype.Type{bsontype.Boolean},
		},
		{
			Name:              "Int32",
			Value:             birch.VC.Int32(42),
			ExpectedCount:     1,
			FirstEncodedValue: 42,
			NumEncodedValues:  1,
			Types:             []bsontype.Type{bsontype.Int32},
		},
		{
			Name:              "Int32Zero",
			Value:             birch.VC.Int32(0),
			ExpectedCount:     1,
			FirstEncodedValue: 0,
			NumEncodedValues:  1,
			Types:             []bsontype.Type{bsontype.Int32},
		},
		{
			Name:              "Int32Negative",
			Value:             birch.VC.Int32(-42),
			ExpectedCount:     1,
			FirstEncodedValue: -42,
			NumEncodedValues:  1,
			Types:             []bsontype.Type{bsontype.Int32},
		},
		{
			Name:              "Int64",
			Value:             birch.VC.Int64(42),
			ExpectedCount:     1,
			FirstEncodedValue: 42,
			NumEncodedValues:  1,
			Types:             []bsontype.Type{bsontype.Int64},
		},
		{
			Name:              "Int64Zero",
			Value:             birch.VC.Int64(0),
			ExpectedCount:     1,
			FirstEncodedValue: 0,
			NumEncodedValues:  1,
			Types:             []bsontype.Type{bsontype.Int64},
		},
		{
			Name:              "Int64Negative",
			Value:             birch.VC.Int64(-42),
			ExpectedCount:     1,
			FirstEncodedValue: -42,
			NumEncodedValues:  1,
			Types:             []bsontype.Type{bsontype.Int64},
		},
		{
			Name:              "DateTimeZero",
			Value:             birch.VC.DateTime(0),
			ExpectedCount:     1,
			FirstEncodedValue: 0,
			NumEncodedValues:  1,
			Types:             []bsontype.Type{bsontype.DateTime},
		},
		{
			Name:              "TimestampZero",
			Value:             birch.VC.Timestamp(0, 0),
			ExpectedCount:     1,
			FirstEncodedValue: 0,
			NumEncodedValues:  2,
			Types:             []bsontype.Type{bsontype.Timestamp, bsontype.Timestamp},
		},
		{
			Name:              "TimestampLarger",
			Value:             birch.VC.Timestamp(42, 42),
			ExpectedCount:     1,
			FirstEncodedValue: 42,
			NumEncodedValues:  2,
			Types:             []bsontype.Type{bsontype.Timestamp, bsontype.Timestamp},
		},
		{
			Name:              "EmptyDocument",
			Value:             birch.EC.SubDocumentFromElements("data").Value(),
			NumEncodedValues:  0,
			FirstEncodedValue: 0,
		},
		{
			Name:              "SingleMetricValue",
			Value:             birch.EC.SubDocumentFromElements("data", birch.EC.Int64("foo", 42)).Value(),
			ExpectedCount:     1,
			NumEncodedValues:  1,
			FirstEncodedValue: 42,
			Types:             []bsontype.Type{bsontype.Int64},
		},
		{
			Name:              "MultiMetricValue",
			Value:             birch.EC.SubDocumentFromElements("data", birch.EC.Int64("foo", 7), birch.EC.Int32("foo", 72)).Value(),
			ExpectedCount:     2,
			NumEncodedValues:  2,
			FirstEncodedValue: 7,
			Types:             []bsontype.Type{bsontype.Int64, bsontype.Int32},
		},
		{
			Name:              "MultiNonMetricValue",
			Value:             birch.EC.SubDocumentFromElements("data", birch.EC.String("foo", "var"), birch.EC.String("bar", "bar")).Value(),
			ExpectedCount:     0,
			NumEncodedValues:  0,
			FirstEncodedValue: 0,
		},
		{
			Name:              "MixedArrayFirstMetrics",
			Value:             birch.EC.SubDocumentFromElements("data", birch.EC.Boolean("zp", true), birch.EC.String("foo", "var"), birch.EC.Int64("bar", 7)).Value(),
			ExpectedCount:     2,
			NumEncodedValues:  2,
			FirstEncodedValue: 1,
			Types:             []bsontype.Type{bsontype.Boolean, bsontype.Int64},
		},
		{
			Name:              "ArraEmptyArray",
			Value:             birch.VC.Array(birch.NewArray()),
			NumEncodedValues:  0,
			FirstEncodedValue: 0,
		},
		{
			Name:              "ArrayWithSingleMetricValue",
			Value:             birch.VC.ArrayFromValues(birch.VC.Int64(42)),
			ExpectedCount:     1,
			NumEncodedValues:  1,
			FirstEncodedValue: 42,
			Types:             []bsontype.Type{bsontype.Int64},
		},
		{
			Name:              "ArrayWithMultiMetricValue",
			Value:             birch.VC.ArrayFromValues(birch.VC.Int64(7), birch.VC.Int32(72)),
			ExpectedCount:     2,
			NumEncodedValues:  2,
			FirstEncodedValue: 7,
			Types:             []bsontype.Type{bsontype.Int64, bsontype.Int32},
		},
		{
			Name:              "ArrayWithMultiNonMetricValue",
			Value:             birch.VC.ArrayFromValues(birch.VC.String("var"), birch.VC.String("bar")),
			NumEncodedValues:  0,
			FirstEncodedValue: 0,
		},
		{
			Name:              "ArrayWithMixedArrayFirstMetrics",
			Value:             birch.VC.ArrayFromValues(birch.VC.Boolean(true), birch.VC.String("var"), birch.VC.Int64(7)),
			NumEncodedValues:  2,
			ExpectedCount:     2,
			FirstEncodedValue: 1,
			Types:             []bsontype.Type{bsontype.Boolean, bsontype.Int64},
		},
		{
			Name:              "DoubleNoTruncate",
			Value:             birch.VC.Double(40.0),
			NumEncodedValues:  1,
			ExpectedCount:     1,
			FirstEncodedValue: 40,
			Types:             []bsontype.Type{bsontype.Double},
		},
		{
			Name:              "DateTime",
			Value:             birch.EC.Time("", now).Value(),
			ExpectedCount:     1,
			FirstEncodedValue: epochMs(now),
			NumEncodedValues:  1,
			Types:             []bsontype.Type{bsontype.DateTime},
		},
	} {
		t.Run(test.Name, func(t *testing.T) {
			metrics, err := extractMetricsFromValue(test.Value)
			assert.NoError(t, err)
			assert.Equal(t, test.NumEncodedValues, len(metrics.values))

			keys, num := testutil.IsMetricsValue("keyname", test.Value)
			if test.NumEncodedValues > 0 {
				assert.EqualValues(t, test.FirstEncodedValue, metrics.values[0].Interface())
				assert.True(t, len(keys) >= 1)
				assert.True(t, strings.HasPrefix(keys[0], "keyname"))
			} else {
				assert.Len(t, keys, 0)
				assert.Zero(t, num)
			}

			require.Len(t, metrics.types, len(test.Types))
			for i := range metrics.types {
				assert.Equal(t, test.Types[i], metrics.types[i])
			}
		})
	}
}

func TestDocumentExtraction(t *testing.T) {
	for _, test := range []struct {
		Name               string
		Document           *birch.Document
		EncoderShouldError bool
		NumEncodedValues   int
		FirstEncodedValue  int64
		Types              []bsontype.Type
	}{
		{
			Name:              "EmptyDocument",
			Document:          birch.NewDocument(),
			NumEncodedValues:  0,
			FirstEncodedValue: 0,
		},
		{
			Name:              "NilDocumentsDocument",
			Document:          (&birch.Document{IgnoreNilInsert: true}).Append(nil, nil),
			NumEncodedValues:  0,
			FirstEncodedValue: 0,
		},
		{
			Name:              "SingleMetricValue",
			Document:          birch.NewDocument(birch.EC.Int64("foo", 42)),
			NumEncodedValues:  1,
			FirstEncodedValue: 42,
			Types:             []bsontype.Type{bsontype.Int64},
		},
		{
			Name:              "MultiMetricValue",
			Document:          birch.NewDocument(birch.EC.Int64("foo", 7), birch.EC.Int32("foo", 72)),
			NumEncodedValues:  2,
			FirstEncodedValue: 7,
			Types:             []bsontype.Type{bsontype.Int64, bsontype.Int32},
		},
		{
			Name:              "MultiNonMetricValue",
			Document:          birch.NewDocument(birch.EC.String("foo", "var"), birch.EC.String("bar", "bar")),
			NumEncodedValues:  0,
			FirstEncodedValue: 0,
		},
		{
			Name:              "MixedArrayFirstMetrics",
			Document:          birch.NewDocument(birch.EC.Boolean("zp", true), birch.EC.String("foo", "var"), birch.EC.Int64("bar", 7)),
			NumEncodedValues:  2,
			FirstEncodedValue: 1,
			Types:             []bsontype.Type{bsontype.Boolean, bsontype.Int64},
		},
	} {
		t.Run(test.Name, func(t *testing.T) {
			metrics, err := extractMetricsFromDocument(test.Document)
			assert.NoError(t, err)
			assert.Equal(t, test.NumEncodedValues, len(metrics.values))
			assert.False(t, metrics.ts.IsZero())
			if len(metrics.values) > 0 {
				assert.EqualValues(t, test.FirstEncodedValue, metrics.values[0].Interface())
			}
			require.Len(t, metrics.types, len(test.Types))
			for i := range metrics.types {
				assert.Equal(t, test.Types[i], metrics.types[i])
			}
		})
	}
}

func TestArrayExtraction(t *testing.T) {
	for _, test := range []struct {
		Name               string
		Array              *birch.Array
		EncoderShouldError bool
		NumEncodedValues   int
		FirstEncodedValue  int64
		Types              []bsontype.Type
	}{
		{
			Name:              "EmptyArray",
			Array:             birch.NewArray(),
			NumEncodedValues:  0,
			FirstEncodedValue: 0,
		},
		{
			Name:              "SingleMetricValue",
			Array:             birch.NewArray(birch.VC.Int64(42)),
			NumEncodedValues:  1,
			FirstEncodedValue: 42,
			Types:             []bsontype.Type{bsontype.Int64},
		},
		{
			Name:              "MultiMetricValue",
			Array:             birch.NewArray(birch.VC.Int64(7), birch.VC.Int32(72)),
			NumEncodedValues:  2,
			FirstEncodedValue: 7,
			Types:             []bsontype.Type{bsontype.Int64, bsontype.Int32},
		},
		{
			Name:              "MultiNonMetricValue",
			Array:             birch.NewArray(birch.VC.String("var"), birch.VC.String("bar")),
			NumEncodedValues:  0,
			FirstEncodedValue: 0,
		},
		{
			Name:              "MixedArrayFirstMetrics",
			Array:             birch.NewArray(birch.VC.Boolean(true), birch.VC.String("var"), birch.VC.Int64(7)),
			NumEncodedValues:  2,
			FirstEncodedValue: 1,
			Types:             []bsontype.Type{bsontype.Boolean, bsontype.Int64},
		},
	} {
		t.Run(test.Name, func(t *testing.T) {
			metrics, err := extractMetricsFromArray(test.Array)
			assert.NoError(t, err)
			assert.Equal(t, test.NumEncodedValues, len(metrics.values))
			if test.NumEncodedValues >= 1 {
				assert.EqualValues(t, test.FirstEncodedValue, metrics.values[0].Interface())
			}
			require.Len(t, metrics.types, len(test.Types))
			for i := range metrics.types {
				assert.Equal(t, test.Types[i], metrics.types[i])
			}
		})
	}
}

func TestMetricsHashValue(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name        string
		value       *birch.Value
		expectedNum int
		keyElems    int
	}{
		{
			name:        "IgnoredType",
			value:       birch.VC.Null(),
			expectedNum: 0,
			keyElems:    0,
		},
		{
			name:        "ObjectID",
			value:       birch.VC.ObjectID(types.NewObjectID()),
			expectedNum: 0,
			keyElems:    0,
		},
		{
			name:        "String",
			value:       birch.VC.String("foo"),
			expectedNum: 0,
			keyElems:    0,
		},
		{
			name:        "Decimal128",
			value:       birch.VC.Decimal128(decimal.NewDecimal128(42, 42)),
			expectedNum: 0,
			keyElems:    0,
		},
		{
			name:        "BoolTrue",
			value:       birch.VC.Boolean(true),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "BoolFalse",
			value:       birch.VC.Boolean(false),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "Int32",
			value:       birch.VC.Int32(42),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "Int32Zero",
			value:       birch.VC.Int32(0),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "Int32Negative",
			value:       birch.VC.Int32(-42),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "Int64",
			value:       birch.VC.Int64(42),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "Int64Zero",
			value:       birch.VC.Int64(0),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "Int64Negative",
			value:       birch.VC.Int64(-142),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "DateTimeZero",
			value:       birch.VC.DateTime(0),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "DateTime",
			value:       birch.EC.Time("", now.Round(time.Second)).Value(),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "TimestampZero",
			value:       birch.VC.Timestamp(0, 0),
			expectedNum: 2,
			keyElems:    1,
		},
		{
			name:        "TimestampLarger",
			value:       birch.VC.Timestamp(42, 42),
			expectedNum: 2,
			keyElems:    1,
		},
		{
			name:        "EmptyDocument",
			value:       birch.EC.SubDocumentFromElements("data").Value(),
			expectedNum: 0,
			keyElems:    0,
		},
		{
			name:        "SingleMetricValue",
			value:       birch.EC.SubDocumentFromElements("data", birch.EC.Int64("foo", 42)).Value(),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "MultiMetricValue",
			value:       birch.EC.SubDocumentFromElements("data", birch.EC.Int64("foo", 7), birch.EC.Int32("foo", 72)).Value(),
			expectedNum: 2,
			keyElems:    2,
		},
		{
			name:        "MultiNonMetricValue",
			value:       birch.EC.SubDocumentFromElements("data", birch.EC.String("foo", "var"), birch.EC.String("bar", "bar")).Value(),
			expectedNum: 0,
			keyElems:    0,
		},
		{
			name:        "MixedArrayFirstMetrics",
			value:       birch.EC.SubDocumentFromElements("data", birch.EC.Boolean("zp", true), birch.EC.String("foo", "var"), birch.EC.Int64("bar", 7)).Value(),
			expectedNum: 2,
			keyElems:    2,
		},
		{
			name:        "ArraEmptyArray",
			value:       birch.VC.Array(birch.NewArray()),
			expectedNum: 0,
			keyElems:    0,
		},
		{
			name:        "ArrayWithSingleMetricValue",
			value:       birch.VC.ArrayFromValues(birch.VC.Int64(42)),
			expectedNum: 1,
			keyElems:    1,
		},
		{
			name:        "ArrayWithMultiMetricValue",
			value:       birch.VC.ArrayFromValues(birch.VC.Int64(7), birch.VC.Int32(72)),
			expectedNum: 2,
			keyElems:    2,
		},
		{
			name:        "ArrayWithMultiNonMetricValue",
			value:       birch.VC.ArrayFromValues(birch.VC.String("var"), birch.VC.String("bar")),
			expectedNum: 0,
			keyElems:    0,
		},
		{
			name:        "ArrayWithMixedArrayFirstMetrics",
			value:       birch.VC.ArrayFromValues(birch.VC.Boolean(true), birch.VC.String("var"), birch.VC.Int64(7)),
			expectedNum: 2,
			keyElems:    2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Run("Legacy", func(t *testing.T) {
				keys, num := testutil.IsMetricsValue("key", test.value)
				assert.Equal(t, test.expectedNum, num)
				assert.Equal(t, test.keyElems, len(keys))
			})
			t.Run("Checksum", func(t *testing.T) {
				assert.Equal(t, test.expectedNum, metricKeyHashValue(fnv.New128(), "key", test.value))
			})
		})
	}
}

func TestMetricsToElement(t *testing.T) {
	for _, test := range []struct {
		name       string
		ref        *birch.Element
		metrics    []Metric
		expected   *birch.Element
		outNum     int
		isDocument bool
	}{
		{
			name: "ObjectID",
			ref:  birch.EC.ObjectID("foo", types.NewObjectID()),
		},
		{
			name: "String",
			ref:  birch.EC.String("foo", "bar"),
		},
		{
			name: "Regex",
			ref:  birch.EC.Regex("foo", "bar", "bar"),
		},
		{
			name: "Decimal128",
			ref:  birch.EC.Decimal128("foo", decimal.NewDecimal128(1, 2)),
		},
		{
			name: "Double",
			ref:  birch.EC.Double("foo", 4.42),
			metrics: []Metric{
				{Values: []int64{normalizeFloat(4.42)}},
			},
			expected: birch.EC.Double("foo", 4.42),
			outNum:   1,
		},
		{
			name: "Short",
			ref:  birch.EC.Int32("foo", 4),
			metrics: []Metric{
				{Values: []int64{37}},
			},
			expected: birch.EC.Int32("foo", 37),
			outNum:   1,
		},
		{

			name: "FalseBool",
			ref:  birch.EC.Boolean("foo", true),
			metrics: []Metric{
				{Values: []int64{0}},
			},
			expected: birch.EC.Boolean("foo", false),
			outNum:   1,
		},
		{

			name: "TrueBool",
			ref:  birch.EC.Boolean("foo", false),
			metrics: []Metric{
				{Values: []int64{1}},
			},
			expected: birch.EC.Boolean("foo", true),
			outNum:   1,
		},
		{

			name: "SuperTrueBool",
			ref:  birch.EC.Boolean("foo", false),
			metrics: []Metric{
				{Values: []int64{100}},
			},
			expected: birch.EC.Boolean("foo", true),
			outNum:   1,
		},
		{

			name:       "EmptyDocument",
			ref:        birch.EC.SubDocument("foo", birch.NewDocument()),
			expected:   birch.EC.SubDocument("foo", birch.NewDocument()),
			isDocument: true,
		},
		{

			name: "DateTimeFromTime",
			ref:  birch.EC.Time("foo", time.Now()),
			metrics: []Metric{
				{Values: []int64{1000}},
			},
			expected: birch.EC.DateTime("foo", 1000),
			outNum:   1,
		},
		{

			name: "DateTime",
			ref:  birch.EC.DateTime("foo", 19999),
			metrics: []Metric{
				{Values: []int64{1000}},
			},
			expected: birch.EC.DateTime("foo", 1000),
			outNum:   1,
		},
		{

			name: "TimeStamp",
			ref:  birch.EC.Timestamp("foo", 19999, 100),
			metrics: []Metric{
				{Values: []int64{1000}},
				{Values: []int64{1000}},
			},
			expected: birch.EC.Timestamp("foo", 1000, 1000),
			outNum:   2,
		},
		{
			name:     "ArrayEmpty",
			ref:      birch.EC.ArrayFromElements("foo", birch.VC.String("foo"), birch.VC.String("bar")),
			expected: birch.EC.Array("foo", birch.NewArray()),
		},
		{
			name: "ArraySingle",
			metrics: []Metric{
				{Values: []int64{1}},
			},
			ref:      birch.EC.ArrayFromElements("foo", birch.VC.Boolean(true)),
			expected: birch.EC.Array("foo", birch.NewArray(birch.VC.Boolean(true))),
			outNum:   1,
		},
		{
			name: "ArrayMulti",
			metrics: []Metric{
				{Values: []int64{1}},
				{Values: []int64{77}},
			},
			ref:      birch.EC.ArrayFromElements("foo", birch.VC.Boolean(true), birch.VC.Int32(33)),
			expected: birch.EC.Array("foo", birch.NewArray(birch.VC.Boolean(true), birch.VC.Int32(77))),
			outNum:   2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			elem, num := restoreElement(test.ref, 0, test.metrics, 0)
			assert.Equal(t, test.outNum, num)
			if !test.isDocument {
				assert.Equal(t, test.expected, elem)
			} else {
				assert.Equal(t, fmt.Sprint(test.expected.Value().Interface()), fmt.Sprint(elem.Value().Interface()))
			}

		})
	}
}

func TestIsOneChecker(t *testing.T) {
	assert.False(t, isNum(1, nil))
	assert.False(t, isNum(1, birch.VC.Int32(32)))
	assert.False(t, isNum(1, birch.VC.Int32(0)))
	assert.False(t, isNum(1, birch.VC.Int64(32)))
	assert.False(t, isNum(1, birch.VC.Int64(0)))
	assert.False(t, isNum(1, birch.VC.Double(32.2)))
	assert.False(t, isNum(1, birch.VC.Double(0.45)))
	assert.False(t, isNum(1, birch.VC.Double(0.0)))
	assert.False(t, isNum(1, birch.VC.String("foo")))
	assert.False(t, isNum(1, birch.VC.Boolean(true)))
	assert.False(t, isNum(1, birch.VC.Boolean(false)))

	assert.True(t, isNum(1, birch.VC.Int32(1)))
	assert.True(t, isNum(1, birch.VC.Int64(1)))
	assert.True(t, isNum(1, birch.VC.Double(1.0)))
}
