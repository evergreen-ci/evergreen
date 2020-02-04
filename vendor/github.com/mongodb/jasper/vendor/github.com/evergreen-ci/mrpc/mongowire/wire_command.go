package mongowire

import (
	"github.com/evergreen-ci/birch"
	"github.com/pkg/errors"
)

func NewCommand(db, name string, args, metadata *birch.Document, inputs []birch.Document) Message {
	return &CommandMessage{
		header: MessageHeader{
			OpCode:    OP_COMMAND,
			RequestID: 19,
		},
		DB:          db,
		CmdName:     name,
		CommandArgs: args,
		Metadata:    metadata,
		InputDocs:   inputs,
	}
}

func (m *CommandMessage) HasResponse() bool     { return true }
func (m *CommandMessage) Header() MessageHeader { return m.header }

func (m *CommandMessage) Scope() *OpScope {
	return &OpScope{
		Type:    m.header.OpCode,
		Context: m.DB,
		Command: m.CmdName,
	}
}

func (m *CommandMessage) Serialize() []byte {
	size := 16 /* header */
	size += len(m.DB) + 1
	size += len(m.CmdName) + 1
	size += getDocSize(m.CommandArgs)
	size += getDocSize(m.Metadata)
	for _, d := range m.InputDocs {
		size += getDocSize(&d)
	}
	m.header.Size = int32(size)

	buf := make([]byte, size)
	m.header.WriteInto(buf)

	loc := int64(16)
	loc += int64(writeCString(m.DB, buf, int(loc)))
	loc += int64(writeCString(m.CmdName, buf, int(loc)))

	offset, _ := m.CommandArgs.WriteDocument(uint(loc), buf)
	loc += offset
	offset, _ = m.Metadata.WriteDocument(uint(loc), buf)
	loc += offset

	for _, d := range m.InputDocs {
		offset, _ = d.WriteDocument(uint(loc), buf)
		loc += offset
	}

	return buf
}

func (h *MessageHeader) parseCommandMessage(buf []byte) (Message, error) {
	var err error

	cmd := &CommandMessage{
		header: *h,
	}

	cmd.DB, err = readCString(buf)
	if err != nil {
		return cmd, err
	}

	if len(buf) < len(cmd.DB)+1 {
		return nil, errors.New("invalid command message -- message length is too short")
	}
	buf = buf[len(cmd.DB)+1:]

	cmd.CmdName, err = readCString(buf)
	if err != nil {
		return nil, err
	}
	if len(buf) < len(cmd.CmdName)+1 {
		return nil, errors.New("invalid command message -- message length is too short")
	}
	buf = buf[len(cmd.CmdName)+1:]

	cmd.CommandArgs, err = birch.ReadDocument(buf)
	if err != nil {
		return nil, err
	}

	size, err := cmd.CommandArgs.Validate()
	if err != nil {
		return nil, err
	}

	if len(buf) < int(size) {
		return cmd, errors.New("invalid command message -- message length is too short")
	}
	buf = buf[size:]

	cmd.Metadata, err = birch.ReadDocument(buf)
	if err != nil {
		return nil, err
	}

	size, err = cmd.Metadata.Validate()
	if err != nil {
		return nil, err
	}

	if len(buf) < int(size) {
		return cmd, errors.New("invalid command message -- message length is too short")
	}
	buf = buf[size:]

	for len(buf) > 0 {
		doc, err := birch.ReadDocument(buf)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		size, err = doc.Validate()
		if err != nil {
			return nil, errors.WithStack(err)
		}

		buf = buf[size:]
		cmd.InputDocs = append(cmd.InputDocs, *doc.Copy())
	}

	return cmd, nil
}
