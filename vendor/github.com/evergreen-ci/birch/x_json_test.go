package birch

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/evergreen-ci/birch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type jsonDocumentTestCase struct {
	Name       string
	ShouldSkip bool
	Doc        *Document
	Expected   string
}

func makeDocumentTestCases(depth int) []jsonDocumentTestCase {
	depth++
	now := time.Now().Round(time.Hour)

	base := []jsonDocumentTestCase{
		{
			Name:     "SimpleString",
			Doc:      DC.Elements(EC.String("hello", "world")),
			Expected: `{"hello":"world"}`,
		},
		{
			Name:     "MultiString",
			Doc:      DC.Elements(EC.String("hello", "world"), EC.String("this", "that")),
			Expected: `{"hello":"world","this":"that"}`,
		},
		{
			Name:     "SimpleTrue",
			Doc:      DC.Elements(EC.Boolean("hello", true)),
			Expected: `{"hello":true}`,
		},
		{
			Name:     "SimpleFalse",
			Doc:      DC.Elements(EC.Boolean("hello", false)),
			Expected: `{"hello":false}`,
		},
		{
			Name:     "ObjectID",
			Doc:      DC.Elements(EC.ObjectID("_id", types.MustObjectIDFromHex("5df67fa01cbe64e51b598f18"))),
			Expected: `{"_id":{"$oid":"5df67fa01cbe64e51b598f18"}}`,
		},
		{
			Name:     "RegularExpression",
			Doc:      DC.Elements(EC.Regex("rex", ".*", "i")),
			Expected: `{"rex":{"$regularExpression":{"pattern":".*","options":"i"}}}`,
		},
		{
			Name:     "SimpleInt",
			Doc:      DC.Elements(EC.Int("hello", 42)),
			Expected: `{"hello":42}`,
		},
		{
			Name:     "MaxKey",
			Doc:      DC.Elements(EC.MaxKey("most")),
			Expected: `{"most":{"$maxKey":1}}`,
		},
		{
			Name:     "MinKey",
			Doc:      DC.Elements(EC.MinKey("most")),
			Expected: `{"most":{"$minKey":1}}`,
		},
		{
			Name:     "Undefined",
			Doc:      DC.Elements(EC.Undefined("kip")),
			Expected: `{"kip":{"$undefined":true}}`,
		},
		{
			Name:     "Code",
			Doc:      DC.Elements(EC.JavaScript("js", "let out = map(function(k, v){})")),
			Expected: `{"js":{"$code":"let out = map(function(k, v){})"}}`,
		},
		{
			Name:     "CodeWithScope",
			Doc:      DC.Elements(EC.CodeWithScope("js", "let out = map(function(k, v){v+a})", DC.Elements(EC.Int("a", 1)))),
			Expected: `{"js":{"$code":"let out = map(function(k, v){v+a})","$scope":{"a":1}}}`,
		},
		{
			Name:     "Symbol",
			Doc:      DC.Elements(EC.Symbol("signified", "signifier")),
			Expected: `{"signified":{"$symbol":"signifier"}}`,
		},
		{

			Name:     "MDBTimeStamp",
			Doc:      DC.Elements(EC.Timestamp("mdbts", uint32(now.Unix()), 1)),
			Expected: fmt.Sprintf(`{"mdbts":{"$timestamp":{"t":%d,"i":1}}}`, now.Unix()),
		},
		{
			Name:     "SimpleInt64",
			Doc:      DC.Elements(EC.Int("hello", math.MaxInt32+2)),
			Expected: `{"hello":2147483649}`,
		},
		{
			Name:     "SimpleTimestamp",
			Doc:      DC.Elements(EC.Time("nowish", now)),
			Expected: fmt.Sprintf(`{"nowish":{"$date":"%s"}}`, now.Format(time.RFC3339)),
		},
		{
			Name: "Mixed",
			Doc: DC.Elements(
				EC.Int("first", 42),
				EC.String("second", "stringval"),
				EC.SubDocumentFromElements("third",
					EC.Boolean("exists", true),
					EC.Null("does_not"),
				),
			),
			Expected: `{"first":42,"second":"stringval","third":{"exists":true,"does_not":null}}`,
		},
	}

	if depth > 2 {
		return base
	}

	for _, at := range makeArrayTestCases(depth) {
		base = append(base, jsonDocumentTestCase{
			Name:       "Array/" + at.Name,
			ShouldSkip: at.ShouldSkip,
			Doc: DC.Elements(
				EC.Boolean("isArray", true),
				EC.Array("array", at.Array),
			),
			Expected: fmt.Sprintf(`{"isArray":true,"array":%s}`, at.Expected),
		})
	}

	for _, vt := range makeValueTestCases(depth) {
		base = append(base, jsonDocumentTestCase{
			Name:       "Value/" + vt.Name,
			ShouldSkip: vt.ShouldSkip,
			Doc: DC.Elements(
				EC.Boolean("isValue", true),
				EC.Value("value", vt.Val),
			),
			Expected: fmt.Sprintf(`{"isValue":true,"value":%s}`, vt.Expected),
		})
	}

	for _, dt := range makeDocumentTestCases(depth) {
		base = append(base, jsonDocumentTestCase{
			Name:       "SubDocument/" + dt.Name,
			ShouldSkip: dt.ShouldSkip,
			Doc: DC.Elements(
				EC.Boolean("isSubDoc", true),
				EC.SubDocument("first", dt.Doc),
				EC.SubDocument("second", dt.Doc),
			),
			Expected: fmt.Sprintf(`{"isSubDoc":true,"first":%s,"second":%s}`, dt.Expected, dt.Expected),
		})
	}
	return base
}

type jsonArrayTestCase struct {
	Name       string
	ShouldSkip bool
	Array      *Array
	Expected   string
}

func makeArrayTestCases(depth int) []jsonArrayTestCase {
	depth++
	base := []jsonArrayTestCase{
		{
			Name:     "Empty",
			Array:    NewArray(),
			Expected: "[]",
		},
		{
			Name:     "Bools",
			Array:    NewArray(VC.Boolean(true), VC.Boolean(true), VC.Boolean(false), VC.Boolean(false)),
			Expected: "[true,true,false,false]",
		},
		{
			Name:     "Strings",
			Array:    NewArray(VC.String("one"), VC.String("two"), VC.String("three")),
			Expected: `["one","two","three"]`,
		},
		{
			Name: "SingleSubDocument",
			Array: NewArray(
				VC.Document(DC.Elements(EC.Int("a", 1))),
			),
			Expected: `[{"a":1}]`,
		},
		{
			Name: "SingleKeyDocuments",
			Array: NewArray(
				VC.Document(DC.Elements(EC.Int("a", 1))),
				VC.Document(DC.Elements(EC.Int("a", 1))),
				VC.Document(DC.Elements(EC.Int("a", 1))),
			),
			Expected: `[{"a":1},{"a":1},{"a":1}]`,
		},
	}

	if depth > 2 {
		return base
	}

	for _, dt := range makeDocumentTestCases(depth) {
		base = append(base, jsonArrayTestCase{
			Name:       "Document/" + dt.Name,
			ShouldSkip: dt.ShouldSkip,
			Array:      NewArray(VC.Document(dt.Doc), VC.Document(dt.Doc), VC.Document(dt.Doc)),
			Expected:   fmt.Sprintf(`[%s,%s,%s]`, dt.Expected, dt.Expected, dt.Expected),
		})
	}

	for _, vt := range makeValueTestCases(depth) {
		base = append(base, jsonArrayTestCase{
			Name:       "Value/" + vt.Name,
			ShouldSkip: vt.ShouldSkip,
			Array:      NewArray(vt.Val, vt.Val, vt.Val),
			Expected:   fmt.Sprintf(`[%s,%s,%s]`, vt.Expected, vt.Expected, vt.Expected),
		})
	}

	for _, at := range makeArrayTestCases(depth + 1) {
		base = append(base, jsonArrayTestCase{
			Name:       "DoubleArray/" + at.Name,
			ShouldSkip: at.ShouldSkip,
			Array:      NewArray(VC.Array(at.Array), VC.Array(at.Array), VC.Array(at.Array)),
			Expected:   fmt.Sprintf(`[%s,%s,%s]`, at.Expected, at.Expected, at.Expected),
		})
	}

	return base
}

type jsonValueTestCase struct {
	Name       string
	ShouldSkip bool
	Val        *Value
	Expected   string
}

func makeValueTestCases(depth int) []jsonValueTestCase {
	depth++
	base := []jsonValueTestCase{
		{
			Name:     "True",
			Val:      VC.Boolean(true),
			Expected: "true",
		},
		{
			Name:     "False",
			Val:      VC.Boolean(false),
			Expected: "false",
		},
		{
			Name:     "Null",
			Val:      VC.Null(),
			Expected: "null",
		},
		{
			Name:     "String",
			Val:      VC.String("helloWorld!"),
			Expected: `"helloWorld!"`,
		},
		{
			Name:     "ObjectID",
			Val:      VC.ObjectID(types.MustObjectIDFromHex("5df67fa01cbe64e51b598f18")),
			Expected: `{"$oid":"5df67fa01cbe64e51b598f18"}`,
		},
		{
			Name:     "SingleKeyDoc",
			Val:      VC.Document(DC.Elements(EC.Int("a", 1))),
			Expected: `{"a":1}`,
		},
	}

	if depth > 2 {
		return base
	}

	for _, docTests := range makeDocumentTestCases(depth) {
		base = append(base, jsonValueTestCase{
			Name:       "Document/" + docTests.Name,
			Val:        VC.Document(docTests.Doc),
			Expected:   docTests.Expected,
			ShouldSkip: docTests.ShouldSkip,
		})
	}

	for _, arrayTests := range makeArrayTestCases(depth) {
		base = append(base, jsonValueTestCase{
			Name:       "Array/" + arrayTests.Name,
			Val:        VC.Array(arrayTests.Array),
			Expected:   arrayTests.Expected,
			ShouldSkip: arrayTests.ShouldSkip,
		})
	}

	return base
}

func TestJSON(t *testing.T) {
	t.Run("Marshal", func(t *testing.T) {
		t.Run("Document", func(t *testing.T) {
			for _, test := range makeDocumentTestCases(0) {
				if test.ShouldSkip {
					continue
				}

				t.Run(test.Name, func(t *testing.T) {
					out, err := test.Doc.MarshalJSON()

					require.NoError(t, err)
					require.Equal(t, test.Expected, string(out))
				})
			}
		})
		t.Run("Array", func(t *testing.T) {
			for _, test := range makeArrayTestCases(0) {
				if test.ShouldSkip {
					continue
				}
				t.Run(test.Name, func(t *testing.T) {
					out, err := test.Array.MarshalJSON()

					require.NoError(t, err)
					require.Equal(t, test.Expected, string(out))
				})
			}
		})
		t.Run("Value", func(t *testing.T) {
			for _, test := range makeValueTestCases(0) {
				if test.ShouldSkip {
					continue
				}

				t.Run(test.Name, func(t *testing.T) {
					out, err := test.Val.MarshalJSON()

					require.NoError(t, err)
					require.Equal(t, test.Expected, string(out))
				})
			}
		})
	})
	t.Run("Unmarshal", func(t *testing.T) {
		t.Run("Document", func(t *testing.T) {
			for _, test := range makeDocumentTestCases(0) {
				if test.ShouldSkip {
					continue
				}

				t.Run(test.Name, func(t *testing.T) {
					doc := DC.New()
					err := doc.UnmarshalJSON([]byte(test.Expected))
					require.NoError(t, err)
					iter := doc.Iterator()
					for iter.Next() {
						elem := iter.Element()
						expected, err := test.Doc.LookupErr(elem.Key())
						require.NoError(t, err)
						assert.True(t, elem.Value().Equal(expected), "[%s] %s != %s",
							test.Expected,
							expected.Interface(), elem.Value().Interface())
					}
					require.NoError(t, iter.Err())
				})
			}
		})
		t.Run("Array", func(t *testing.T) {
			for _, test := range makeArrayTestCases(0) {
				if test.ShouldSkip {
					continue
				}

				t.Run(test.Name, func(t *testing.T) {
					array := NewArray()
					err := array.UnmarshalJSON([]byte(test.Expected))
					require.NoError(t, err)

					iter := array.Iterator()
					idx := uint(0)
					for iter.Next() {
						elem := iter.Value()
						expected, err := test.Array.LookupErr(idx)
						require.NoError(t, err)
						assert.True(t, elem.Equal(expected))
						idx++
					}
					require.NoError(t, iter.Err())
				})
			}
		})
		t.Run("Value", func(t *testing.T) {
			for _, test := range makeValueTestCases(0) {
				if test.ShouldSkip {
					continue
				}

				t.Run(test.Name, func(t *testing.T) {
					value := &Value{}
					err := value.UnmarshalJSON([]byte(test.Expected))
					require.NoError(t, err)

					assert.True(t, value.Equal(test.Val))
				})
			}
		})

	})
}
