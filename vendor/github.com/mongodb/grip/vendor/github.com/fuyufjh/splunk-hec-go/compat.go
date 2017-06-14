package hec

// functions and constants added for MAKE-191 to be compatible for go 1.4 and gccgo.

const (
	ioSeekStart    = 0 // seek relative to the origin of the file
	ioSeekCurrent  = 1 // seek relative to the current offset
	ioSeekEnd      = 2 // seek relative to the end
	httpMethodPost = "POST"
)

// lastIndexByte returns the index of the last instance of c in s, or -1 if c is not present in s.
func lastIndexByte(s []byte, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}
