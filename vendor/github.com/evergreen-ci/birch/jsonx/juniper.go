package jsonx

type Document struct {
	elems []*Element
}

func (d *Document) Append(elems ...*Element) *Document { d.elems = append(d.elems, elems...); return d }
func (d *Document) Len() int                           { return len(d.elems) }
func (d *Document) Iterator() Iterator                 { return &documentIterImpl{doc: d} }
func (d *Document) Copy() *Document {
	nd := DC.Make(d.Len())
	for _, elem := range d.elems {
		nd.Append(elem.Copy())
	}
	return nd
}

func (d *Document) KeyAtIndex(idx int) string {
	elem := d.ElementAtIndex(idx)
	if elem == nil {
		return ""
	}

	return elem.Key()
}

func (d *Document) ElementAtIndex(idx int) *Element {
	if idx+1 > d.Len() {
		return nil
	}

	return d.elems[idx]
}

type Array struct {
	elems []*Value
}

func (a *Array) Append(vals ...*Value) *Array { a.elems = append(a.elems, vals...); return a }
func (a *Array) Len() int                     { return len(a.elems) }
func (a *Array) Iterator() Iterator           { return &arrayIterImpl{array: a} }

func (a *Array) Copy() *Array {
	na := AC.Make(a.Len())
	for _, elem := range a.elems {
		na.Append(elem.Copy())
	}
	return na
}

type Element struct {
	key   string
	value *Value
}

func (e *Element) Key() string             { return e.key }
func (e *Element) Value() *Value           { return e.value }
func (e *Element) ValueOK() (*Value, bool) { return e.value, e.value != nil }
func (e *Element) Copy() *Element          { return EC.Value(e.key, e.value.Copy()) }

type Value struct {
	t     Type
	value interface{}
}

func (v *Value) Type() Type                    { return v.t }
func (v *Value) Interface() interface{}        { return v.value }
func (v *Value) StringValue() string           { return v.value.(string) }
func (v *Value) Array() *Array                 { return v.value.(*Array) }
func (v *Value) Document() *Document           { return v.value.(*Document) }
func (v *Value) Boolean() bool                 { return v.value.(bool) }
func (v *Value) Int() int                      { return v.value.(int) }
func (v *Value) Float64() float64              { return v.value.(float64) }
func (v *Value) StringValueOK() (string, bool) { out, ok := v.value.(string); return out, ok }
func (v *Value) ArrayOK() (*Array, bool)       { out, ok := v.value.(*Array); return out, ok }
func (v *Value) DocumentOK() (*Document, bool) { out, ok := v.value.(*Document); return out, ok }
func (v *Value) BooleanOK() (bool, bool)       { out, ok := v.value.(bool); return out, ok }
func (v *Value) IntOK() (int, bool)            { out, ok := v.value.(int); return out, ok }
func (v *Value) Float64OK() (float64, bool)    { out, ok := v.value.(float64); return out, ok }

func (v *Value) Copy() *Value {
	return &Value{
		t:     v.t,
		value: v.value,
	}
}
