package jsonx

type Iterator interface {
	Err() error
	Next() bool
	Element() *Element
	Value() *Value
}

type documentIterImpl struct {
	idx     int
	doc     *Document
	current *Element
	err     error
}

func (iter *documentIterImpl) Next() bool {
	if iter.idx+1 > iter.doc.Len() {
		return false
	}

	iter.current = iter.doc.elems[iter.idx].Copy()
	iter.idx++
	return true
}

func (iter *documentIterImpl) Element() *Element { return iter.current }
func (iter *documentIterImpl) Value() *Value     { return iter.current.value }
func (iter *documentIterImpl) Err() error        { return nil }

type arrayIterImpl struct {
	idx     int
	array   *Array
	current *Value
	err     error
}

func (iter *arrayIterImpl) Next() bool {
	if iter.idx+1 > iter.array.Len() {
		return false
	}

	iter.current = iter.array.elems[iter.idx].Copy()
	iter.idx++
	return true
}

func (iter *arrayIterImpl) Element() *Element { return &Element{value: iter.current} }
func (iter *arrayIterImpl) Value() *Value     { return iter.current }
func (iter *arrayIterImpl) Err() error        { return nil }
