package mongowire

import (
	"github.com/evergreen-ci/birch"
	"github.com/pkg/errors"
)

func NewDelete(ns string, flags int32, filter *birch.Document) Message {
	return &deleteMessage{
		header: MessageHeader{
			RequestID: 19,
			OpCode:    OP_DELETE,
		},
		Namespace: ns,
		Flags:     flags,
		Filter:    filter,
	}
}

func (m *deleteMessage) HasResponse() bool     { return false }
func (m *deleteMessage) Header() MessageHeader { return m.header }
func (m *deleteMessage) Scope() *OpScope       { return &OpScope{Type: m.header.OpCode, Context: m.Namespace} }
func (m *deleteMessage) Serialize() []byte {
	size := 16 /* header */ + 8 /* update header */
	size += len(m.Namespace) + 1
	size += int(getDocSize(m.Filter))

	m.header.Size = int32(size)

	buf := make([]byte, size)
	m.header.WriteInto(buf)

	loc := 16
	loc += writeInt32(0, buf, loc)

	loc += writeCString(m.Namespace, buf, loc)

	loc += writeInt32(m.Flags, buf, loc)

	loc += writeDocAt(m.Filter, buf, loc)

	return buf
}

func (h *MessageHeader) parseDeleteMessage(buf []byte) (Message, error) {
	var (
		err error
		loc int
	)

	m := &deleteMessage{
		header: *h,
	}

	if len(buf) < 4 {
		return nil, errors.New("invalid delete message -- message must have length of at least 4 bytes")
	}
	m.Reserved = readInt32(buf[loc:])
	loc += 4

	m.Namespace, err = readCString(buf[loc:])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	loc += len(m.Namespace) + 1

	if len(buf) < loc+4 {
		return m, errors.New("invalid delete message -- message length is too short")
	}
	m.Flags = readInt32(buf[loc:])
	loc += 4

	m.Filter, err = birch.ReadDocument(buf[loc:])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	loc += int(getDocSize(m.Filter)) // nolint

	return m, nil
}
