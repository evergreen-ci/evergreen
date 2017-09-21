package db

import mgo "gopkg.in/mgo.v2"

// NewCombinedIterator produces a DocumentIterator that is an
// mgo.Iter, with a modified Close() method that also closes the
// provided mgo session after closing the iterator.
func NewCombinedIterator(ses Session, iter Iterator) Iterator {
	c := CombinedCloser{
		Iterator: iter,
	}

	session, ok := ses.(sessionWrapper)
	if ok {
		c.ses = session.Session
	}

	return c
}

type CombinedCloser struct {
	Iterator
	ses *mgo.Session
}

func (c CombinedCloser) Close() error {
	err := c.Iterator.Close()

	if c.ses != nil {
		c.ses.Close()
	}

	return err
}
