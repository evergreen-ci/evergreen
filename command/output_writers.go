package command

import ()

// generic implementations of io.Writer

// A trivial implementation of io.Writer that saves the last thing written.
// Useful for some testing situations where a line of stdout or stderr needs
// to be checked.
type CacheLastWritten struct {
	LastWritten []byte
}

func (self *CacheLastWritten) Write(p []byte) (int, error) {
	self.LastWritten = p
	return len(p), nil
}
