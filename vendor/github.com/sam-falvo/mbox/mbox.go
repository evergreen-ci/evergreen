// vim: ts=8 ai noexpandtab

package mbox

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// MboxStream objects represent sequential streams of e-mail messages that
// exist in the MBOX format.
type MboxStream struct {
	prefetch       []byte
	prefetchLength int
	currentLine    int
	r              *bufio.Reader
}

// The ReadMessage method parses the input for another complete message.  A
// message consists of a From header, at least one header, followed by a
// collection of lines of text corresponding to the body of the message.
//
// If no From marker exists, we either don't have an MBOX file, a corrupted
// MBOX file, or we're at the end of the input stream.  Giving the benefit of
// the doubt, this package returns io.EOF for an error in any of these
// situations.
//
// After reading a message, you must read the body of the message prior to
// reading the next.  Otherwise, a framing error will cause the reader to
// return io.EOF prematurely.  See the SkippingTheBody example for a simple
// example showing how to do this simply.
func (m *MboxStream) ReadMessage() (msg *Message, err error) {
	msg = &Message{
		mbox:    m,
		headers: make(map[string][]string, 0),
	}

	msg.sendingAddress, err = m.parseFrom()
	if err != nil {
		msg = nil
		return
	}

	msg.headers, err = m.parseHeaders()
	if err != nil {
		msg = nil
		return
	}

	err = m.parseBlankLine()
	if err != nil {
		msg = nil
		return
	}

	return
}

// errorf provides an error object whose string also includes the line-number.
// TODO(sfalvo): This prevents the user from testing for specific error responses.
// Find a better way of exposing the line on which an error occurs.
func (m *MboxStream) errorf(format string, args ...interface{}) error {
	s := fmt.Sprintf(format, args...)
	return fmt.Errorf("%d:%s", m.currentLine, s)
}

// parseBlankLine will succeed only if the current line of the mbox file is a
// blank line.  Blank lines are required by the MBOX format conventions to separate
// MIME headers from message content.
func (m *MboxStream) parseBlankLine() error {
	if (len(m.prefetch) > 1) || (m.prefetch[0] != '\n') {
		return m.errorf("Blank line expected")
	}
	return m.nextLine()
}

// parseFrom will succeed only if the current line of the mbox file is a properly
// formed "From " separator.  It will extract the sending e-mail address from this
// line.  If this line doesn't exist, it yields an error instead.
func (m *MboxStream) parseFrom() (who string, err error) {
	who, err = extractSendingAddress(m)
	if err == nil {
		err = m.nextLine()
	}
	return
}

func extractSendingAddress(m *MboxStream) (who string, err error) {
	if string(m.prefetch[0:5]) != "From " {
		return "", io.EOF
	}
	if m.prefetchLength < 6 {
		return "", m.errorf("Sender address expected")
	}
	who = strings.TrimSpace(string(m.prefetch[5:]))
	if who == "" {
		return "", m.errorf("Sender address cannot be whitespace")
	}
	return
}

// parseHeaders will read in the headers from the mbox file.  It builds a
// mapping from string to an array of strings.  Each header key corresponds to
// one or more strings as received in the mbox file.  For greatest fidelity,
// leading whitespace on continued lines is preserved.
func (m *MboxStream) parseHeaders() (hs map[string][]string, err error) {
	hs = make(map[string][]string, 0)
	for {
		key, values, err := m.parseHeader()
		if err != nil {
			return nil, err
		}
		hs[key] = values
		if m.prefetch[0] == '\n' {
			break
		}
	}
	return hs, nil
}

// parseHeader will read in a single header from the mbox file.
// Header attributes start with a "key: value" syntax; however, continued
// lines thereafter just start with some flavor of whitespace.
func (m *MboxStream) parseHeader() (key string, values []string, err error) {
	// Headers consist of a key, a colon, and a value.  The key must not be an empty string.
	// Therefore, the smallest possible header is K:, which takes up two characters.
	if m.prefetchLength < 2 {
		return "", nil, m.errorf("Header attribute expected")
	}

	if isspace(m.prefetch[0]) {
		return "", nil, m.errorf("Unexpected continuation of a header somehow missed")
	}

	k := strings.Index(string(m.prefetch), ":")
	if k < 1 {
		return "", nil, m.errorf("Colon not found in expected 'key: value' syntax")
	}

	key = string(m.prefetch[0:k])
	values = []string{strings.TrimSpace(string(m.prefetch[k+1:]))}
	err = m.nextLine()
	if err != nil {
		return "", nil, err
	}

	for {
		// Continuation lines consist of at least one whitespace and at least one regular character.
		if (m.prefetchLength < 2) || (!isspace(m.prefetch[0])) {
			break
		}
		continuation := strings.TrimRight(string(m.prefetch), " \r\n\t\b\v")
		values = append(values, continuation)
		err = m.nextLine()
		if err != nil {
			return "", nil, err
		}
	}

	return
}

func isspace(b byte) bool {
	return b < 33
}

// CreateMboxStream decorates an io.Reader instance with an mbox parser.
// It will produce an io.EOF if the file doesn't appear to be an mbox-formatted file.
// It determines this by verifying the first five characters of the file matches "From " (note the space).
// Observe, however, that CreateMboxStream() succeeding does not imply that it actually is a correctly formatted mbox file.
func CreateMboxStream(s io.Reader) (m *MboxStream, err error) {
	m = &MboxStream{
		prefetch: make([]byte, 1000),
		r:        bufio.NewReader(s),
	}

	err = m.nextLine()
	if err != nil {
		m = nil
		return
	}

	_, err = extractSendingAddress(m)
	return
}

// nextLine retrieves the next logical line from the mbox file.  The caller
// should be concerned with one of three cases:
//
// - A successful read yields no error.
// - Attempting to read past the end of the input stream yields io.EOF.
// - All other errors are reported as necessary.
func (m *MboxStream) nextLine() error {
	slice, err := m.r.ReadSlice('\n')
	if err != nil {
		return err
	}
	m.prefetch = m.prefetch[0:len(slice)]
	copy(m.prefetch, slice)
	m.prefetchLength = len(m.prefetch)
	m.currentLine++
	return nil
}
