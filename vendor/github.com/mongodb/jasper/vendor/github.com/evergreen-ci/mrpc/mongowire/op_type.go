package mongowire

type OpType int32

const (
	OP_REPLY         OpType = 1
	OP_MSG           OpType = 1000
	OP_UPDATE        OpType = 2001
	OP_INSERT        OpType = 2002
	RESERVED         OpType = 2003
	OP_QUERY         OpType = 2004
	OP_GET_MORE      OpType = 2005
	OP_DELETE        OpType = 2006
	OP_KILL_CURSORS  OpType = 2007
	OP_COMMAND       OpType = 2010
	OP_COMMAND_REPLY OpType = 2011
)

func (op OpType) String() string {
	switch op {
	case OP_REPLY:
		return "OP_REPLY"
	case OP_MSG:
		return "OP_MSG"
	case OP_UPDATE:
		return "OP_UPDATE"
	case OP_INSERT:
		return "OP_INSERT"
	case RESERVED:
		return "RESERVED"
	case OP_QUERY:
		return "OP_QUERY"
	case OP_GET_MORE:
		return "OP_GET_MORE"
	case OP_DELETE:
		return "OP_DELETE"
	case OP_KILL_CURSORS:
		return "OP_KILL_CURSORS"
	case OP_COMMAND:
		return "OP_COMMAND"
	case OP_COMMAND_REPLY:
		return "OP_COMMAND_REPLY"
	default:
		return ""
	}
}
