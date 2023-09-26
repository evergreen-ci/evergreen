package log

type emptyIterator struct{}

// EmptyIterator returns a convenience log iterator with no data.
func EmptyIterator() *emptyIterator { return &emptyIterator{} }

func (*emptyIterator) Next() bool { return false }

func (*emptyIterator) Item() LogLine { return LogLine{} }

func (*emptyIterator) Exhausted() bool { return true }

func (*emptyIterator) Err() error { return nil }

func (*emptyIterator) Close() error { return nil }
