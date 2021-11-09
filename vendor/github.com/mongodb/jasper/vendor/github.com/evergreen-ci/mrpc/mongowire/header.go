package mongowire

import (
	"github.com/pkg/errors"
)

type MessageHeader struct {
	Size       int32 // total message size
	RequestID  int32
	ResponseTo int32
	OpCode     OpType
}

func (h *MessageHeader) WriteInto(buf []byte) {
	_ = writeInt32(h.Size, buf, 0)
	_ = writeInt32(h.RequestID, buf, 4)
	_ = writeInt32(h.ResponseTo, buf, 8)
	_ = writeInt32(int32(h.OpCode), buf, 12)
}

func (h *MessageHeader) Parse(body []byte) (Message, error) {
	var (
		m   Message
		err error
	)

	switch h.OpCode {
	case OP_REPLY:
		m, err = h.parseReplyMessage(body)
	case OP_UPDATE:
		m, err = h.parseUpdateMessage(body)
	case OP_INSERT:
		m, err = h.parseInsertMessage(body)
	case OP_QUERY:
		m, err = h.parseQueryMessage(body)
	case OP_GET_MORE:
		m, err = h.parseGetMoreMessage(body)
	case OP_DELETE:
		m, err = h.parseDeleteMessage(body)
	case OP_KILL_CURSORS:
		m, err = h.parseKillCursorsMessage(body)
	case OP_COMMAND:
		m, err = h.parseCommandMessage(body)
	case OP_COMMAND_REPLY:
		m, err = h.parseCommandReplyMessage(body)
	case OP_MSG:
		m, err = h.parseMsgBody(body)
	default:
		return nil, errors.Errorf("unknown op code: %s", h.OpCode)
	}

	return m, errors.WithStack(err)
}
