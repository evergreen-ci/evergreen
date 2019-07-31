package mongowire

import "github.com/pkg/errors"

func NewKillCursors(ids ...int64) Message {
	return &killCursorsMessage{
		header: MessageHeader{
			RequestID: 19,
			OpCode:    OP_KILL_CURSORS,
		},
		NumCursors: int32(len(ids)),
		CursorIds:  ids,
	}
}

func (m *killCursorsMessage) HasResponse() bool     { return false }
func (m *killCursorsMessage) Header() MessageHeader { return m.header }
func (m *killCursorsMessage) Scope() *OpScope       { return &OpScope{Type: m.header.OpCode} }

func (m *killCursorsMessage) Serialize() []byte {
	size := 16 /* header */ + 8 /* header */ + (8 * int(m.NumCursors))

	m.header.Size = int32(size)

	buf := make([]byte, size)
	m.header.WriteInto(buf)

	writeInt32(0, buf, 16)
	writeInt32(m.NumCursors, buf, 20)

	loc := 24

	for _, c := range m.CursorIds {
		writeInt64(c, buf, loc)
		loc += 8
	}

	return buf
}

func (h *MessageHeader) parseKillCursorsMessage(buf []byte) (Message, error) {
	if len(buf) < 8 {
		return nil, errors.New("invalid kill cursors message -- message must have length of at least 8 bytes")
	}

	loc := 0
	m := &killCursorsMessage{
		header: *h,
	}

	m.Reserved = readInt32(buf)
	loc += 4

	m.NumCursors = readInt32(buf[loc:])
	loc += 4

	if len(buf[loc:]) < int(m.NumCursors)*8 {
		return nil, errors.Errorf("invalid kill cursors message -- NumCursors = %d is larger than number of cursors in message", m.NumCursors)
	}

	m.CursorIds = make([]int64, int(m.NumCursors))

	for i := 0; i < int(m.NumCursors); i++ {
		m.CursorIds[i] = readInt64(buf[loc:])
		loc += 8
	}

	return m, nil
}
