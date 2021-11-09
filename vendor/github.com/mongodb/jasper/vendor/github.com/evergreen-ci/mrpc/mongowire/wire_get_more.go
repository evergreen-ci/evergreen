package mongowire

import (
	"github.com/pkg/errors"
)

func NewGetMore(ns string, number int32, cursorID int64) Message {
	return &getMoreMessage{
		header: MessageHeader{
			RequestID: 19,
			OpCode:    OP_GET_MORE,
		},
		Namespace: ns,
		NReturn:   number,
		CursorId:  cursorID,
	}
}

func (m *getMoreMessage) HasResponse() bool     { return true }
func (m *getMoreMessage) Header() MessageHeader { return m.header }

func (m *getMoreMessage) Scope() *OpScope {
	return &OpScope{
		Type:    m.header.OpCode,
		Context: m.Namespace,
	}
}

func (m *getMoreMessage) Serialize() []byte {
	size := 16 /* header */ + 16 /* query header */
	size += len(m.Namespace) + 1

	m.header.Size = int32(size)

	buf := make([]byte, size)
	m.header.WriteInto(buf)

	loc := 16
	loc += writeInt32(0, buf, loc)

	loc += writeCString(m.Namespace, buf, loc)
	loc += writeInt32(m.NReturn, buf, loc)

	loc += writeInt64(m.CursorId, buf, loc)

	return buf
}

func (h *MessageHeader) parseGetMoreMessage(buf []byte) (Message, error) {
	var (
		err error
		loc int
	)

	if len(buf) < 4 {
		return nil, errors.New("invalid get more message -- message must have length of at least 4 bytes")
	}

	qm := &getMoreMessage{
		header: *h,
	}

	qm.Reserved = readInt32(buf)
	loc += 4

	qm.Namespace, err = readCString(buf[loc:])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	loc += len(qm.Namespace) + 1

	if len(buf) < loc+12 {
		return nil, errors.New("invalid get more message -- message length is too short")
	}
	qm.NReturn = readInt32(buf[loc:])
	loc += 4

	qm.CursorId = readInt64(buf[loc:])
	loc += 8 // nolint

	return qm, nil
}
