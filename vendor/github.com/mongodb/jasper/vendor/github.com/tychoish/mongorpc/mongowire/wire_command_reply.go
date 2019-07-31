package mongowire

import (
	"github.com/pkg/errors"
	"github.com/tychoish/mongorpc/bson"
)

func (m *commandReplyMessage) HasResponse() bool     { return false }
func (m *commandReplyMessage) Header() MessageHeader { return m.header }
func (m *commandReplyMessage) Scope() *OpScope       { return nil }

func (m *commandReplyMessage) Serialize() []byte {
	size := 16 /* header */
	size += int(m.CommandReply.Size)
	size += int(m.Metadata.Size)
	for _, d := range m.OutputDocs {
		size += int(d.Size)
	}
	m.header.Size = int32(size)

	buf := make([]byte, size)
	m.header.WriteInto(buf)

	loc := 16

	m.CommandReply.Copy(&loc, buf)
	m.Metadata.Copy(&loc, buf)

	for _, d := range m.OutputDocs {
		d.Copy(&loc, buf)
	}

	return buf
}

func (h *MessageHeader) parseCommandReplyMessage(buf []byte) (Message, error) {
	rm := &commandReplyMessage{
		header: *h,
	}

	var err error

	rm.CommandReply, err = bson.ParseSimple(buf)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(buf) < int(rm.CommandReply.Size) {
		return nil, errors.New("invalid command message -- message length is too short")
	}
	buf = buf[rm.CommandReply.Size:]

	rm.Metadata, err = bson.ParseSimple(buf)
	if err != nil {
		return nil, err
	}
	if len(buf) < int(rm.Metadata.Size) {
		return nil, errors.New("invalid command message -- message length is too short")
	}
	buf = buf[rm.Metadata.Size:]

	for len(buf) > 0 {
		doc, err := bson.ParseSimple(buf)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		buf = buf[doc.Size:]
		rm.OutputDocs = append(rm.OutputDocs, doc)
	}

	return rm, nil
}
