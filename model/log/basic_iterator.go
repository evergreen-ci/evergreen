package log

type basicIterator struct {
	i      int
	items  []LogLine
	closed bool
}

// newBasicIterator returns a LogIterator that iterates over in-memory log
// lines.
func newBasicIterator(items []LogLine) *basicIterator {
	return &basicIterator{
		i:     -1,
		items: items,
	}
}

func (it *basicIterator) Next() bool {
	if it.closed || it.Exhausted() {
		return false
	}

	it.i++

	return true
}

func (it *basicIterator) Item() LogLine {
	if len(it.items) > 0 {
		return it.items[it.i]
	}

	return LogLine{}
}

func (it *basicIterator) Exhausted() bool { return it.i == len(it.items)-1 }

func (*basicIterator) Err() error { return nil }

func (it *basicIterator) Close() error {
	it.closed = true

	return nil
}
