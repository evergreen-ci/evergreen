package jsonx

import (
	"encoding/json"

	"github.com/evergreen-ci/birch/jsonx/internal"
	"github.com/pkg/errors"
)

func (d *Document) UnmarshalJSON(in []byte) error {
	res, err := internal.ParseBytes(in)
	if err != nil {
		return errors.Wrap(err, "problem parsing raw json")
	}

	if !res.IsObject() {
		return errors.New("cannot unmarshal values or arrays into Documents")
	}

	res.ForEach(func(key, value internal.Result) bool {
		var val *Value
		val, err = getValueForResult(value)
		if err != nil {
			return false
		}

		d.Append(EC.Value(key.Str, val))
		return true
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (a *Array) UnmarshalJSON(in []byte) error {
	res, err := internal.ParseBytes(in)
	if err != nil {
		return errors.Wrap(err, "problem parsing raw json")
	}

	if !res.IsArray() {
		return errors.New("cannot unmarshal a non-arrays into an array")
	}

	for _, item := range res.Array() {
		val, err := getValueForResult(item)
		if err != nil {
			return errors.WithStack(err)
		}

		a.Append(val)
	}

	return nil
}

func (v *Value) UnmarshalJSON(in []byte) error {
	res, err := internal.ParseBytes(in)
	if err != nil {
		return errors.Wrap(err, "problem parsing raw json")
	}

	out, err := getValueForResult(res)
	if err != nil {
		return errors.WithStack(err)
	}

	v.value = out.value
	v.t = out.t
	return nil
}

///////////////////////////////////
//
// Internal

func getValueForResult(value internal.Result) (*Value, error) {
	switch {
	case value.Type == internal.String:
		return VC.String(value.Str), nil
	case value.Type == internal.Null:
		return VC.Nil(), nil
	case value.Type == internal.True:
		return VC.Boolean(true), nil
	case value.Type == internal.False:
		return VC.Boolean(false), nil
	case value.Type == internal.Number:
		num := json.Number(value.String())
		if igr, err := num.Int64(); err == nil {
			return VC.Int(int(igr)), nil
		} else if df, err := num.Float64(); err == nil {
			return VC.Float64(df), nil
		}

		return nil, errors.Errorf("number value [%s] is invalid [%+v]", value.Str, value)
	case value.IsArray():
		source := value.Array()
		array := AC.Make(len(source))
		for _, elem := range source {
			val, err := getValueForResult(elem)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			array.Append(val)
		}

		return VC.Array(array), nil
	case value.IsObject():
		var err error
		doc := DC.New()
		value.ForEach(func(key, value internal.Result) bool {
			val, err := getValueForResult(value)
			if err != nil {
				err = errors.Wrapf(err, "problem with subdocument at key %s", key.Str)
				return false
			}
			doc.Append(EC.Value(key.Str, val))
			return true
		})
		if err != nil {
			return nil, errors.WithStack(err)
		}

		return VC.Object(doc), nil
	default:
		return nil, errors.Errorf("unknown json value type '%s'", value.Type)
	}
}
