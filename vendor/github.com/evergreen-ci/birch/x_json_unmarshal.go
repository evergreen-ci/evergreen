package birch

import (
	"time"

	"github.com/evergreen-ci/birch/decimal"
	"github.com/evergreen-ci/birch/jsonx"
	"github.com/evergreen-ci/birch/types"
	"github.com/pkg/errors"
)

// UnmarshalJSON converts the contents of a document to JSON
// recursively, preserving the order of keys and the rich types from
// bson using MongoDB's extended JSON format for BSON types that have
// no equivalent in JSON.
//
// The underlying document is not emptied before this operation, which
// for non-empty documents could result in duplicate keys.
func (d *Document) UnmarshalJSON(in []byte) error {
	jdoc, err := jsonx.DC.BytesErr(in)
	if err != nil {
		return errors.WithStack(err)
	}
	iter := jdoc.Iterator()
	for iter.Next() {
		elem, err := convertJSONElements(iter.Element())
		if err != nil {
			return errors.WithStack(err)
		}

		d.Append(elem)
	}

	return nil
}

func (a *Array) UnmarshalJSON(in []byte) error {
	ja, err := jsonx.AC.BytesErr(in)
	if err != nil {
		return errors.WithStack(err)
	}
	iter := ja.Iterator()
	for iter.Next() {
		elem, err := convertJSONElements(iter.Element())
		if err != nil {
			return errors.WithStack(err)
		}

		a.Append(elem.value)
	}

	return nil
}

func (v *Value) UnmarshalJSON(in []byte) error {
	va, err := jsonx.VC.BytesErr(in)
	if err != nil {
		return errors.WithStack(err)
	}

	elem, err := convertJSONElements(jsonx.EC.Value("", va))
	if err != nil {
		return errors.WithStack(err)
	}
	v.Set(elem.Value())
	return nil
}

func convertJSONElements(in *jsonx.Element) (*Element, error) {
	inv := in.Value()
	switch inv.Type() {
	case jsonx.String:
		val, ok := inv.StringValueOK()
		if !ok {
			return nil, errors.New("mismatched json type")
		}
		return EC.String(in.Key(), val), nil
	case jsonx.Bool:
		val, ok := inv.BooleanOK()
		if !ok {
			return nil, errors.New("mismatched json type")
		}
		return EC.Boolean(in.Key(), val), nil
	case jsonx.Null:
		return EC.Null(in.Key()), nil
	case jsonx.NumberInteger:
		val, ok := inv.IntOK()
		if !ok {
			return nil, errors.New("mismatched json type")
		}
		return EC.Int(in.Key(), val), nil
	case jsonx.NumberDouble:
		val, ok := inv.Float64OK()
		if !ok {
			return nil, errors.New("mismatched json type")
		}
		return EC.Double(in.Key(), val), nil
	case jsonx.Number:
		return EC.Interface(in.Key(), inv.Interface()), nil
	case jsonx.ObjectValue:
		indoc := in.Value().Document()
		switch indoc.KeyAtIndex(0) {
		case "$minKey":
			return EC.MinKey(in.Key()), nil
		case "$maxKey":
			return EC.MaxKey(in.Key()), nil
		case "$numberDecimal":
			val, err := decimal.ParseDecimal128(indoc.ElementAtIndex(0).Value().StringValue())
			if err != nil {
				return nil, errors.WithStack(err)
			}

			return EC.Decimal128(in.Key(), val), nil
		case "$timestamp":
			var (
				t   int64
				i   int64
				val int
				ok  bool
			)

			tsDoc := indoc.ElementAtIndex(0).Value().Document()
			iter := tsDoc.Iterator()
			count := 0
			for iter.Next() {
				if count >= 3 {
					break
				}
				elem := iter.Element()

				switch elem.Key() {
				case "t":
					val, ok = elem.Value().IntOK()
					if !ok {
						return nil, errors.Errorf("problem decoding number for timestamp at %s [%T]", in.Key(), elem.Value().Interface())
					}
					t = int64(val)
				case "i":
					val, ok = elem.Value().IntOK()
					if !ok {
						return nil, errors.Errorf("problem decoding number for timestamp at %s [%T]", in.Key(), elem.Value().Interface())
					}
					i = int64(val)
				}
				count++
			}

			return EC.Timestamp(in.Key(), uint32(t), uint32(i)), nil
		case "$symbol":
			return EC.Symbol(in.Key(), indoc.ElementAtIndex(0).Value().StringValue()), nil
		case "$code":
			js, ok := indoc.ElementAtIndex(0).Value().StringValueOK()
			if !ok {
				return nil, errors.New("invalid code document")
			}

			if second := indoc.KeyAtIndex(1); second == "" {
				return EC.JavaScript(in.Key(), js), nil
			} else if second == "$scope" {
				scope, err := convertJSONElements(indoc.ElementAtIndex(1))
				if err != nil {
					return nil, errors.WithStack(err)
				}

				return EC.CodeWithScope(in.Key(), js, scope.Value().MutableDocument()), nil
			} else {
				return nil, errors.Errorf("invalid key '%s' in code with scope for %s", second, in.Key())
			}
		case "$dbPointer":
			var (
				ns  string
				oid string
				ok  bool
			)
			debref := indoc.ElementAtIndex(0).Value().Document()
			iter := debref.Iterator()
			count := 0
			for iter.Next() {
				if count >= 2 {
					break
				}
				elem := iter.Element()

				switch elem.Key() {
				case "$ref":
					ns, ok = elem.Value().StringValueOK()
					if !ok {
						return nil, errors.Errorf("problem decoding ns for dbref in %s", in.Key())
					}
				case "$id":
					oid, ok = elem.Value().StringValueOK()
					if !ok {
						return nil, errors.Errorf("problem decoding ns for oid in %s", in.Key())
					}
				}
				count++
			}
			if ns == "" || oid == "" {
				return nil, errors.New("values for dbref are not defined")
			}

			oidp, err := types.ObjectIDFromHex(oid)
			if err != nil {
				return nil, errors.Wrapf(err, "problem parsing oid from dbref at %s", in.Key())
			}

			return EC.DBPointer(in.Key(), ns, oidp), nil
		case "$regularExpression":
			var (
				pattern string
				options string
				ok      bool
			)
			rex := indoc.ElementAtIndex(0).Value().Document()
			iter := rex.Iterator()
			count := 0
			for iter.Next() {
				if count >= 2 {
					break
				}
				elem := iter.Element()

				switch elem.Key() {
				case "pattern":
					pattern, ok = elem.Value().StringValueOK()
					if !ok {
						return nil, errors.Errorf("problem decoding ns for dbref in %s", in.Key())
					}
				case "options":
					options, ok = elem.Value().StringValueOK()
					if !ok {
						return nil, errors.Errorf("problem decoding ns for oid in %s", in.Key())
					}
				}
				count++
			}

			return EC.Regex(in.Key(), pattern, options), nil
		case "$date":
			date, err := time.Parse(time.RFC3339, indoc.ElementAtIndex(0).Value().StringValue())
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return EC.Time(in.Key(), date), nil
		case "$oid":
			oid, err := types.ObjectIDFromHex(indoc.ElementAtIndex(0).Value().StringValue())
			if err != nil {
				return nil, errors.WithStack(err)
			}

			return EC.ObjectID(in.Key(), oid), nil
		case "$undefined":
			return EC.Undefined(in.Key()), nil
		case "$binary":
			return EC.Binary(in.Key(), []byte(indoc.ElementAtIndex(0).Value().StringValue())), nil
		default:
			iter := indoc.Iterator()

			doc := DC.Make(indoc.Len())
			for iter.Next() {
				elem, err := convertJSONElements(iter.Element())
				if err != nil {
					return nil, errors.WithStack(err)
				}

				doc.Append(elem)
			}
			return EC.SubDocument(in.Key(), doc), nil
		}
	case jsonx.ArrayValue:
		ina := inv.Array()
		iter := ina.Iterator()

		array := MakeArray(ina.Len())
		for iter.Next() {
			elem, err := convertJSONElements(iter.Element())
			if err != nil {
				return nil, errors.WithStack(err)
			}

			array.Append(elem.value)
		}
		return EC.Array(in.Key(), array), nil
	default:
		return nil, errors.Errorf("unknown value type '%s' [%v]", inv.Type(), inv.Interface())
	}
}
