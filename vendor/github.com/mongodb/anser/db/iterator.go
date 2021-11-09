package db

// NewCombinedIterator produces a DocumentIterator that is an
// mgo.Iter, with a modified Close() method that also closes the
// provided mgo session after closing the iterator.
func NewCombinedIterator(ses Session, iter Iterator) Iterator {
	c := CombinedCloser{
		Iterator: iter,
	}

	return c
}

type CombinedCloser struct {
	Iterator
}

func (c CombinedCloser) Close() error {
	err := c.Iterator.Close()

	return err
}
