package mongowire

import (
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/mrpc/model"
	"github.com/pkg/errors"
)

type OpMessageSection interface {
	Type() uint8
	Name() string
	DB() string
	Documents() []birch.Document
	Serialize() []byte
}

const (
	OpMessageSectionBody             = 0
	OpMessageSectionDocumentSequence = 1
)

type opMessagePayloadType0 struct {
	Document *birch.Document
}

func (p *opMessagePayloadType0) Type() uint8 { return OpMessageSectionBody }

func (p *opMessagePayloadType0) Name() string {
	return p.Document.ElementAt(0).Key()
}

func (p *opMessagePayloadType0) DB() string {
	key, err := p.Document.LookupErr("$db")
	if err != nil {
		return ""
	}

	val, ok := key.StringValueOK()
	if !ok {
		return ""
	}

	return val
}

func (p *opMessagePayloadType0) Documents() []birch.Document {
	return []birch.Document{*p.Document.Copy()}
}

func (p *opMessagePayloadType0) Serialize() []byte {
	buf := make([]byte, 1+getDocSize(p.Document))
	buf[0] = p.Type() // kind
	_ = writeDocAt(p.Document, buf, 1)
	return buf
}

type opMessagePayloadType1 struct {
	Size       int32
	Identifier string
	Payload    []birch.Document
}

func (p *opMessagePayloadType1) Type() uint8 { return OpMessageSectionDocumentSequence }

func (p *opMessagePayloadType1) Name() string { return p.Identifier }

func (p *opMessagePayloadType1) DB() string { return "" }

func (p *opMessagePayloadType1) Documents() []birch.Document { return p.Payload }

func (p *opMessagePayloadType1) Serialize() []byte {
	size := 1                     // kind
	size += 4                     // size
	size += len(p.Identifier) + 1 // identifier
	for _, doc := range p.Payload {
		size += getDocSize(&doc)
	}
	p.Size = int32(size)

	buf := make([]byte, size)
	buf[0] = p.Type() // kind
	loc := 1
	loc += writeInt32(p.Size, buf, loc)
	loc += writeCString(p.Identifier, buf, loc)
	for _, doc := range p.Payload { // payload
		loc += writeDocAt(&doc, buf, loc)
	}
	return buf
}

func (m *OpMessage) Header() MessageHeader { return m.header }
func (m *OpMessage) HasResponse() bool     { return m.Flags > 1 }

func (m *OpMessage) Scope() *OpScope {
	var cmd string
	var db string
	// OP_MSG is expected to have exactly one body section.
	for _, section := range m.Items {
		if _, ok := section.(*opMessagePayloadType0); ok {
			cmd = section.Name()
			db = section.DB()
			break
		}
	}
	return &OpScope{
		Type:    m.header.OpCode,
		Context: db,
		Command: cmd,
	}
}

func (m *OpMessage) Serialize() []byte {
	if len(m.serialized) > 0 {
		return m.serialized
	}

	size := 16 // header
	size += 4  // flags
	sections := []byte{}
	for _, section := range m.Items {
		sections = append(sections, section.Serialize()...)
		switch p := section.(type) {
		case *opMessagePayloadType0:
			size += 1 // kind
			size += getDocSize(p.Document)
		case *opMessagePayloadType1:
			size += int(p.Size)
		}
	}
	if m.Checksum != 0 && (m.Flags&1) == 1 {
		size += 4
	}
	m.header.Size = int32(size)
	buf := make([]byte, size)
	m.header.WriteInto(buf)

	loc := 16 // header
	loc += writeInt32(int32(m.Flags), buf, loc)

	copy(buf[loc:], sections)
	loc += len(sections)

	if m.Checksum != 0 && (m.Flags&1) == 1 {
		loc += writeInt32(m.Checksum, buf, loc)
	}

	m.serialized = buf
	return buf
}

func NewOpMessage(moreToCome bool, documents []birch.Document, items ...model.SequenceItem) Message {
	msg := &OpMessage{
		header: MessageHeader{
			OpCode:    OP_MSG,
			RequestID: 19,
		},
		Items: make([]OpMessageSection, len(documents)),
	}

	for idx := range documents {
		msg.Items[idx] = &opMessagePayloadType0{
			Document: documents[idx].Copy(),
		}
	}

	if moreToCome {
		msg.Flags = msg.Flags & 1
	}

	for idx := range items {
		item := items[idx]
		it := &opMessagePayloadType1{
			Identifier: item.Identifier,
		}
		for jdx := range item.Documents {
			it.Payload = append(it.Payload, *item.Documents[jdx].Copy())
			it.Size += int32(getDocSize(&item.Documents[jdx]))
		}
		msg.Items = append(msg.Items, it)
	}

	return msg
}

func (h *MessageHeader) parseMsgBody(body []byte) (Message, error) {
	if len(body) < 4 {
		return nil, errors.New("invalid op message - message must have length of at least 4 bytes")
	}

	msg := &OpMessage{
		header: *h,
	}

	loc := 0
	msg.Flags = uint32(readInt32(body[loc:]))
	loc += 4
	checksumPresent := (msg.Flags & 1) == 1

	for loc < len(body)-4 {
		kind := int32(body[loc])
		loc++

		var err error
		switch kind {
		case OpMessageSectionBody:
			section := &opMessagePayloadType0{}
			docSize := int(readInt32(body[loc:]))
			section.Document, err = birch.ReadDocument(body[loc : loc+docSize])
			loc += getDocSize(section.Document)
			msg.Items = append(msg.Items, section)
		case OpMessageSectionDocumentSequence:
			section := &opMessagePayloadType1{}
			section.Size = readInt32(body[loc:])
			loc += 4
			section.Identifier, err = readCString(body[loc:])
			if err != nil {
				return nil, errors.Wrap(err, "could not read identifier")
			}
			loc += len(section.Identifier) + 1 // c string null terminator

			for remaining := int(section.Size) - 1 - 4 - len(section.Identifier) - 1; remaining > 0; {
				docSize := int(readInt32(body[loc:]))
				doc, err := birch.ReadDocument(body[loc : loc+docSize])
				if err != nil {
					return nil, errors.Wrap(err, "could not read payload document")
				}
				section.Payload = append(section.Payload, *doc.Copy())
				remaining -= docSize
				loc += docSize
			}
			msg.Items = append(msg.Items, section)
		default:
			return nil, errors.Errorf("unrecognized kind bit %d", kind)
		}
	}

	if checksumPresent && loc == len(body)-4 {
		msg.Checksum = readInt32(body[loc:])
		loc += 4
	}

	msg.header.Size = int32(loc)

	return msg, nil
}
