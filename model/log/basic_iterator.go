package log

type basicIterator struct {
	idx       int
	items     []LogLine
	exhausted bool
	closed    bool
}

// newBasicIterator returns a LogIterator that iterates over in-memory log
// lines.
func newBasicIterator(items []LogLine) *basicIterator {
	return &basicIterator{
		idx:   -1,
		items: items,
	}
}

func (it *basicIterator) Next() bool {
	if it.exhausted || it.closed {
		return false
	}

	if it.idx == len(it.items)-1 {
		it.exhausted = true
		return false
	}
	it.idx++

	return true
}

func (it *basicIterator) Item() LogLine {
	if len(it.items) > 0 && it.idx >= 0 {
		return it.items[it.idx]
	}

	return LogLine{}
}

func (it *basicIterator) Exhausted() bool { return it.exhausted }

func (*basicIterator) Err() error { return nil }

func (it *basicIterator) Close() error {
	it.closed = true

	return nil
}
