package mongowire

import (
	"github.com/pkg/errors"
	"github.com/tychoish/mongorpc/bson"
)

func NewCommand(db, name string, args, metadata bson.Simple, inputs []bson.Simple) Message {
	return &commandMessage{
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

func (m *commandMessage) HasResponse() bool     { return true }
func (m *commandMessage) Header() MessageHeader { return m.header }

func (m *commandMessage) Scope() *OpScope {
	return &OpScope{
		Type:    m.header.OpCode,
		Context: m.DB,
		Command: m.CmdName,
	}
}

func (m *commandMessage) Serialize() []byte {
	size := 16 /* header */
	size += len(m.DB) + 1
	size += len(m.CmdName) + 1
	size += int(m.CommandArgs.Size)
	size += int(m.Metadata.Size)
	for _, d := range m.InputDocs {
		size += int(d.Size)
	}
	m.header.Size = int32(size)

	buf := make([]byte, size)
	m.header.WriteInto(buf)

	loc := 16

	writeCString(m.DB, buf, &loc)
	writeCString(m.CmdName, buf, &loc)
	m.CommandArgs.Copy(&loc, buf)
	m.Metadata.Copy(&loc, buf)

	for _, d := range m.InputDocs {
		d.Copy(&loc, buf)
	}

	return buf
}

func (h *MessageHeader) parseCommandMessage(buf []byte) (Message, error) {
	var err error

	cmd := &commandMessage{
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

	cmd.CommandArgs, err = bson.ParseSimple(buf)
	if err != nil {
		return nil, err
	}
	if len(buf) < int(cmd.CommandArgs.Size) {
		return cmd, errors.New("invalid command message -- message length is too short")
	}
	buf = buf[cmd.CommandArgs.Size:]

	cmd.Metadata, err = bson.ParseSimple(buf)
	if err != nil {
		return nil, err
	}
	if len(buf) < int(cmd.Metadata.Size) {
		return cmd, errors.New("invalid command message -- message length is too short")
	}
	buf = buf[cmd.Metadata.Size:]

	for len(buf) > 0 {
		doc, err := bson.ParseSimple(buf)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		buf = buf[doc.Size:]
		cmd.InputDocs = append(cmd.InputDocs, doc)
	}

	return cmd, nil
}
