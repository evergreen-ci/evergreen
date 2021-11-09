package mongowire

import (
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/mrpc/model"
)

type Message interface {
	Header() MessageHeader
	Serialize() []byte
	HasResponse() bool
	Scope() *OpScope
}

// OP_REPLY
type ReplyMessage struct {
	header MessageHeader

	Flags          int32
	CursorId       int64
	StartingFrom   int32
	NumberReturned int32

	Docs []birch.Document
}

// OP_UPDATE
type updateMessage struct {
	header MessageHeader

	Reserved  int32
	Flags     int32
	Namespace string

	Filter *birch.Document
	Update *birch.Document
}

// OP_QUERY
type queryMessage struct {
	header MessageHeader

	Flags     int32
	Skip      int32
	NReturn   int32
	Namespace string

	Query   *birch.Document
	Project *birch.Document
}

// OP_GET_MORE
type getMoreMessage struct {
	header MessageHeader

	Reserved  int32
	NReturn   int32
	CursorId  int64
	Namespace string
}

// OP_INSERT
type insertMessage struct {
	header MessageHeader

	Flags     int32
	Namespace string

	Docs []birch.Document
}

// OP_DELETE
type deleteMessage struct {
	header MessageHeader

	Reserved  int32
	Flags     int32
	Namespace string

	Filter *birch.Document
}

// OP_KILL_CURSORS
type killCursorsMessage struct {
	header MessageHeader

	Reserved   int32
	NumCursors int32
	CursorIds  []int64
}

// OP_COMMAND
type CommandMessage struct {
	header MessageHeader

	DB          string
	CmdName     string
	CommandArgs *birch.Document
	Metadata    *birch.Document
	InputDocs   []birch.Document

	// internal bookekeeping
	upconverted bool
}

// OP_COMMAND_REPLY
type CommandReplyMessage struct {
	header MessageHeader

	CommandReply *birch.Document
	Metadata     *birch.Document
	OutputDocs   []birch.Document
}

// OP_MSG
type OpMessage struct {
	header     MessageHeader
	serialized []byte

	Flags      uint32
	DB         string
	Collection string
	Operation  string
	Items      []OpMessageSection
	Checksum   int32
}

func GetModel(msg Message) (interface{}, OpType) {
	switch m := msg.(type) {
	case *CommandMessage:
		return &model.Command{
			DB:                 m.DB,
			Command:            m.CmdName,
			Arguments:          m.CommandArgs,
			Metadata:           m.Metadata,
			Inputs:             m.InputDocs,
			ConvertedFromQuery: m.upconverted,
		}, OP_COMMAND
	case *OpMessage:
		op := &model.Message{
			Database:   m.DB,
			Collection: m.Collection,
			Operation:  m.Operation,
		}

		switch m.Flags {
		case 0:
			op.Checksum = true
		case 1:
			op.MoreToCome = true
		case 3:
			op.Checksum = true
			op.MoreToCome = true
		}

		for _, section := range m.Items {
			op.Items = append(op.Items, model.SequenceItem{
				Identifier: section.Name(),
				Documents:  section.Documents(),
			})
		}

		return op, OP_MSG
	case *deleteMessage:
		return &model.Delete{
			Namespace: m.Namespace,
			Filter:    m.Filter,
		}, OP_DELETE
	case *insertMessage:
		return &model.Insert{
			Namespace: m.Namespace,
			Documents: m.Docs,
		}, OP_INSERT
	case *queryMessage:
		return &model.Query{
			Namespace: m.Namespace,
			Skip:      m.Skip,
			NReturn:   m.NReturn,
			Query:     m.Query,
			Project:   m.Project,
		}, OP_QUERY
	case *updateMessage:
		update := &model.Update{
			Namespace: m.Namespace,
			Filter:    m.Filter,
			Update:    m.Update,
		}

		switch m.Flags {
		case 1:
			update.Upsert = true
		case 2:
			update.Multi = true
		case 3:
			update.Upsert = true
			update.Multi = true
		}

		return update, OP_UPDATE
	case *ReplyMessage:
		reply := &model.Reply{
			StartingFrom: m.StartingFrom,
			CursorID:     m.CursorId,
			Contents:     m.Docs,
		}

		switch m.Flags {
		case 1:
			reply.QueryFailure = true
		case 0:
			reply.CursorNotFound = true
		}

		return reply, OP_REPLY
	default:
		return nil, OpType(0)
	}
}
