package jsonx

import (
	"io"
	"io/ioutil"

	"github.com/pkg/errors"
)

type DocumentConstructor struct{}

var DC = DocumentConstructor{}

func (DocumentConstructor) New() *Document            { return DC.Make(0) }
func (DocumentConstructor) Make(n int) *Document      { return &Document{elems: make([]*Element, 0, n)} }
func (DocumentConstructor) Bytes(in []byte) *Document { return docConstructorOrPanic(DC.BytesErr(in)) }

func (DocumentConstructor) Reader(in io.Reader) *Document {
	return docConstructorOrPanic(DC.ReaderErr(in))
}

func (DocumentConstructor) Elements(elems ...*Element) *Document {
	return DC.Make(len(elems)).Append(elems...)
}

func (DocumentConstructor) BytesErr(in []byte) (*Document, error) {
	d := DC.New()

	if err := d.UnmarshalJSON(in); err != nil {
		return nil, err
	}

	return d, nil
}

func (DocumentConstructor) ReaderErr(in io.Reader) (*Document, error) {
	buf, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return DC.BytesErr(buf)
}

type ArrayConstructor struct{}

var AC = ArrayConstructor{}

func (ArrayConstructor) New() *Array                     { return AC.Make(0) }
func (ArrayConstructor) Make(n int) *Array               { return &Array{elems: make([]*Value, 0, n)} }
func (ArrayConstructor) Elements(elems ...*Value) *Array { return AC.Make(len(elems)).Append(elems...) }
func (ArrayConstructor) Bytes(in []byte) *Array          { return arrayConstructorOrPanic(AC.BytesErr(in)) }
func (ArrayConstructor) Reader(in io.Reader) *Array      { return arrayConstructorOrPanic(AC.ReaderErr(in)) }

func (ArrayConstructor) BytesErr(in []byte) (*Array, error) {
	a := AC.New()
	if err := a.UnmarshalJSON(in); err != nil {
		return nil, errors.WithStack(err)
	}

	return a, nil
}

func (ArrayConstructor) ReaderErr(in io.Reader) (*Array, error) {
	buf, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return AC.BytesErr(buf)
}

type ElementConstructor struct{}

var EC = ElementConstructor{}

func (ElementConstructor) Value(key string, val *Value) *Element {
	return &Element{key: key, value: val}
}

func (ElementConstructor) String(key string, value string) *Element {
	return EC.Value(key, VC.String(value))
}

func (ElementConstructor) Boolean(key string, val bool) *Element {
	return EC.Value(key, VC.Boolean(val))
}

func (ElementConstructor) Int(key string, n int) *Element {
	return EC.Value(key, VC.Int(n))
}

func (ElementConstructor) Int32(key string, n int32) *Element {
	return EC.Value(key, VC.Int32(n))
}

func (ElementConstructor) Int64(key string, n int64) *Element {
	return EC.Value(key, VC.Int64(n))
}

func (ElementConstructor) Float64(key string, n float64) *Element {
	return EC.Value(key, VC.Float64(n))
}

func (ElementConstructor) Float32(key string, n float32) *Element {
	return EC.Value(key, VC.Float32(n))
}

func (ElementConstructor) Nil(key string) *Element {
	return EC.Value(key, VC.Nil())
}

func (ElementConstructor) Object(key string, doc *Document) *Element {
	return EC.Value(key, VC.Object(doc))
}

func (ElementConstructor) ObjectFromElements(key string, elems ...*Element) *Element {
	return EC.Value(key, VC.ObjectFromElements(elems...))
}

func (ElementConstructor) Array(key string, a *Array) *Element {
	return EC.Value(key, VC.Array(a))
}

func (ElementConstructor) ArrayFromElements(key string, elems ...*Value) *Element {
	return EC.Value(key, VC.ArrayFromElements(elems...))
}

type ValueConstructor struct{}

var VC = ValueConstructor{}

func (ValueConstructor) Bytes(in []byte) *Value { return valueConstructorOrPanic(VC.BytesErr(in)) }
func (ValueConstructor) BytesErr(in []byte) (*Value, error) {
	val := &Value{}

	if err := val.UnmarshalJSON(in); err != nil {
		return nil, errors.WithStack(err)
	}
	return val, nil
}

func (ValueConstructor) Reader(in io.Reader) *Value { return valueConstructorOrPanic(VC.ReaderErr(in)) }
func (ValueConstructor) ReaderErr(in io.Reader) (*Value, error) {
	buf, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return VC.BytesErr(buf)
}

func (ValueConstructor) String(s string) *Value {
	return &Value{
		t:     String,
		value: s,
	}
}

func (ValueConstructor) Int(n int) *Value {
	return &Value{
		t:     NumberInteger,
		value: n,
	}
}

func (ValueConstructor) Int32(n int32) *Value {
	return &Value{
		t:     NumberInteger,
		value: n,
	}
}

func (ValueConstructor) Int64(n int64) *Value {
	return &Value{
		t:     NumberInteger,
		value: n,
	}
}

func (ValueConstructor) Float64(n float64) *Value {
	return &Value{
		t:     NumberDouble,
		value: n,
	}
}

func (ValueConstructor) Float32(n float32) *Value {
	return &Value{
		t:     NumberDouble,
		value: n,
	}
}

func (ValueConstructor) Nil() *Value {
	return &Value{
		t:     Null,
		value: nil,
	}
}

func (ValueConstructor) Boolean(b bool) *Value {
	return &Value{
		t:     Bool,
		value: b,
	}
}

func (ValueConstructor) Object(doc *Document) *Value {
	return &Value{
		t:     ObjectValue,
		value: doc,
	}
}

func (ValueConstructor) ObjectFromElements(elems ...*Element) *Value {
	return VC.Object(DC.Elements(elems...))
}

func (ValueConstructor) Array(a *Array) *Value {
	return &Value{
		t:     ArrayValue,
		value: a,
	}
}

func (ValueConstructor) ArrayFromElements(elems ...*Value) *Value {
	return VC.Array(AC.Elements(elems...))
}
