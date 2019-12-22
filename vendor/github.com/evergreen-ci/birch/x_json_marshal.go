package birch

import (
	"time"

	"github.com/evergreen-ci/birch/bsontype"
	"github.com/evergreen-ci/birch/jsonx"
)

// MarshalJSON produces a JSON representation of the Document,
// preserving the order of the keys, and type information for types
// that have no JSON equivlent using MongoDB's extended JSON format
// where needed.
func (d *Document) MarshalJSON() ([]byte, error) { return d.toJSON().MarshalJSON() }

func (d *Document) toJSON() *jsonx.Document {
	iter := d.Iterator()
	out := jsonx.DC.Make(d.Len())
	for iter.Next() {
		elem := iter.Element()
		out.Append(jsonx.EC.Value(elem.Key(), elem.Value().toJSON()))
	}
	if iter.Err() != nil {
		return nil
	}

	return out
}

// MarshalJSON produces a JSON representation of an Array preserving
// the type information for the types that have no JSON equivalent
// using MongoDB's extended JSON format where needed.
func (a *Array) MarshalJSON() ([]byte, error) { return a.toJSON().MarshalJSON() }

func (a *Array) toJSON() *jsonx.Array {
	iter := a.Iterator()
	out := jsonx.AC.Make(a.Len())
	for iter.Next() {
		out.Append(iter.Value().toJSON())
	}
	if iter.Err() != nil {
		panic(iter.Err())
		return nil
	}

	return out
}

func (v *Value) MarshalJSON() ([]byte, error) { return v.toJSON().MarshalJSON() }

func (v *Value) toJSON() *jsonx.Value {
	switch v.Type() {
	case bsontype.Double:
		return jsonx.VC.Float64(v.Double())
	case bsontype.String:
		return jsonx.VC.String(v.StringValue())
	case bsontype.EmbeddedDocument:
		return jsonx.VC.Object(v.MutableDocument().toJSON())
	case bsontype.Array:
		return jsonx.VC.Array(v.MutableArray().toJSON())
	case bsontype.Binary:
		t, d := v.Binary()

		return jsonx.VC.ObjectFromElements(
			jsonx.EC.ObjectFromElements("$binary",
				jsonx.EC.String("base64", string(t)),
				jsonx.EC.String("subType", string(d)),
			),
		)
	case bsontype.Undefined:
		return jsonx.VC.ObjectFromElements(jsonx.EC.Boolean("$undefined", true))
	case bsontype.ObjectID:
		return jsonx.VC.ObjectFromElements(jsonx.EC.String("$oid", v.ObjectID().Hex()))
	case bsontype.Boolean:
		return jsonx.VC.Boolean(v.Boolean())
	case bsontype.DateTime:
		return jsonx.VC.ObjectFromElements(jsonx.EC.String("$date", v.Time().Format(time.RFC3339)))
	case bsontype.Null:
		return jsonx.VC.Nil()
	case bsontype.Regex:
		pattern, opts := v.Regex()

		return jsonx.VC.ObjectFromElements(
			jsonx.EC.ObjectFromElements("$regularExpression",
				jsonx.EC.String("pattern", pattern),
				jsonx.EC.String("options", opts),
			),
		)
	case bsontype.DBPointer:
		ns, oid := v.DBPointer()

		return jsonx.VC.ObjectFromElements(
			jsonx.EC.ObjectFromElements("$dbPointer",
				jsonx.EC.String("$ref", ns),
				jsonx.EC.String("$id", oid.Hex()),
			),
		)
	case bsontype.JavaScript:
		return jsonx.VC.ObjectFromElements(jsonx.EC.String("$code", v.JavaScript()))
	case bsontype.Symbol:
		return jsonx.VC.ObjectFromElements(jsonx.EC.String("$symbol", v.Symbol()))
	case bsontype.CodeWithScope:
		code, scope := v.MutableJavaScriptWithScope()

		return jsonx.VC.ObjectFromElements(
			jsonx.EC.String("$code", code),
			jsonx.EC.Object("$scope", scope.toJSON()),
		)
	case bsontype.Int32:
		return jsonx.VC.Int32(v.Int32())
	case bsontype.Timestamp:
		t, i := v.Timestamp()

		return jsonx.VC.ObjectFromElements(
			jsonx.EC.ObjectFromElements("$timestamp",
				jsonx.EC.Int64("t", int64(t)),
				jsonx.EC.Int64("i", int64(i)),
			),
		)
	case bsontype.Int64:
		return jsonx.VC.Int64(v.Int64())
	case bsontype.Decimal128:
		return jsonx.VC.ObjectFromElements(jsonx.EC.String("$numberDecimal", v.Decimal128().String()))
	case bsontype.MinKey:
		return jsonx.VC.ObjectFromElements(jsonx.EC.Int("$minKey", 1))
	case bsontype.MaxKey:
		return jsonx.VC.ObjectFromElements(jsonx.EC.Int("$maxKey", 1))
	default:
		return nil
	}
}
