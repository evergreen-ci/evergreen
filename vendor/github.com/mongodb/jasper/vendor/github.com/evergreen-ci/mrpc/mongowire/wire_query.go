package mongowire

import (
	"github.com/evergreen-ci/birch"
	"github.com/pkg/errors"
)

func NewQuery(ns string, flags, skip, toReturn int32, query, project *birch.Document) Message {
	return &queryMessage{
		header: MessageHeader{
			RequestID: 19,
			OpCode:    OP_QUERY,
		},
		Flags:     flags,
		Namespace: ns,
		Skip:      skip,
		NReturn:   toReturn,
		Query:     query,
		Project:   project,
	}
}

func (m *queryMessage) HasResponse() bool     { return true }
func (m *queryMessage) Header() MessageHeader { return m.header }
func (m *queryMessage) Scope() *OpScope       { return &OpScope{Type: m.header.OpCode, Context: m.Namespace} }

func (m *queryMessage) Serialize() []byte {
	size := 16 /* header */ + 12 /* query header */
	size += len(m.Namespace) + 1
	size += getDocSize(m.Query)
	size += getDocSize(m.Project)

	m.header.Size = int32(size)

	buf := make([]byte, size)
	m.header.WriteInto(buf)

	loc := 16
	loc += writeInt32(m.Flags, buf, loc)

	loc += writeCString(m.Namespace, buf, loc)
	loc += writeInt32(m.Skip, buf, loc)

	loc += writeInt32(m.NReturn, buf, loc)

	loc += writeDocAt(m.Query, buf, loc)
	loc += writeDocAt(m.Project, buf, loc)

	return buf
}

func (m *queryMessage) convertToCommand() *CommandMessage {
	if !NamespaceIsCommand(m.Namespace) {
		return nil
	}

	return &CommandMessage{
		header: MessageHeader{
			OpCode:    OP_COMMAND,
			RequestID: 19,
		},
		DB:          NamespaceToDB(m.Namespace),
		CmdName:     m.Query.ElementAt(0).Key(),
		CommandArgs: m.Query,
		upconverted: true,
	}
}

func (h *MessageHeader) parseQueryMessage(buf []byte) (Message, error) {
	if len(buf) < 4 {
		return nil, errors.New("invalid query message -- message must have length of at least 4 bytes")
	}

	var (
		loc int
		err error
	)

	qm := &queryMessage{
		header: *h,
	}

	qm.Flags = readInt32(buf)
	loc += 4

	qm.Namespace, err = readCString(buf[loc:])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	loc += len(qm.Namespace) + 1

	if len(buf) < loc+8 {
		return qm, errors.New("invalid query message -- message length is too short")
	}
	qm.Skip = readInt32(buf[loc:])
	loc += 4

	qm.NReturn = readInt32(buf[loc:])
	loc += 4

	qm.Query, err = birch.ReadDocument(buf[loc:])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	loc += getDocSize(qm.Query)

	if loc < len(buf) {
		qm.Project, err = birch.ReadDocument(buf[loc:])
		if err != nil {
			return nil, errors.WithStack(err)
		}
		loc += getDocSize(qm.Project) // nolint
	}

	if NamespaceIsCommand(qm.Namespace) {
		return qm.convertToCommand(), nil
	}

	return qm, nil
}
