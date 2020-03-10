package mongowire

import (
	"github.com/evergreen-ci/birch"
	"github.com/pkg/errors"
)

func NewCommandReply(reply, metadata *birch.Document, output []birch.Document) Message {
	return &CommandReplyMessage{
		header: MessageHeader{
			OpCode:    OP_COMMAND_REPLY,
			RequestID: 19,
		},
		CommandReply: reply,
		Metadata:     metadata,
		OutputDocs:   output,
	}
}

func (m *CommandReplyMessage) HasResponse() bool     { return false }
func (m *CommandReplyMessage) Header() MessageHeader { return m.header }
func (m *CommandReplyMessage) Scope() *OpScope       { return nil }

func (m *CommandReplyMessage) Serialize() []byte {
	size := 16 /* header */

	size += getDocSize(m.CommandReply)
	size += getDocSize(m.Metadata)
	for _, d := range m.OutputDocs {
		size += getDocSize(&d)
	}
	m.header.Size = int32(size)

	buf := make([]byte, size)
	m.header.WriteInto(buf)

	loc := 16
	loc += writeDocAt(m.CommandReply, buf, loc)
	loc += writeDocAt(m.Metadata, buf, loc)

	for _, d := range m.OutputDocs {
		loc += writeDocAt(&d, buf, loc)
	}

	return buf
}

func (h *MessageHeader) parseCommandReplyMessage(buf []byte) (Message, error) {
	rm := &CommandReplyMessage{
		header: *h,
	}

	var err error

	rm.CommandReply, err = birch.ReadDocument(buf)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	replySize := getDocSize(rm.CommandReply)
	if len(buf) < replySize {
		return nil, errors.New("invalid command message -- message length is too short")
	}
	buf = buf[replySize:]

	rm.Metadata, err = birch.ReadDocument(buf)
	if err != nil {
		return nil, err
	}
	metaSize := getDocSize(rm.Metadata)
	if len(buf) < metaSize {
		return nil, errors.New("invalid command message -- message length is too short")
	}
	buf = buf[metaSize:]

	for len(buf) > 0 {
		doc, err := birch.ReadDocument(buf)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		buf = buf[getDocSize(doc):]
		rm.OutputDocs = append(rm.OutputDocs, *doc.Copy())
	}

	return rm, nil
}
