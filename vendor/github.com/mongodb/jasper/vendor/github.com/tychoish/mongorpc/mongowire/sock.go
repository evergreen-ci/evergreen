package mongowire

import (
	"io"

	"github.com/pkg/errors"
)

const MaxInt32 = 2147483647

func ReadMessage(reader io.Reader) (Message, error) {
	// read header
	sizeBuf := make([]byte, 4)
	n, err := reader.Read(sizeBuf)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if n != 4 {
		return nil, errors.Errorf("didn't read message size from socket, got %d", n)
	}

	header := MessageHeader{}

	header.Size = readInt32(sizeBuf)

	if header.Size > int32(200*1024*1024) {
		if header.Size == 542393671 {
			return nil, errors.Errorf("message too big, probably http request %d", header.Size)
		}
		return nil, errors.Errorf("message too big %d", header.Size)
	}

	if header.Size < 0 || header.Size-4 > MaxInt32 {
		return nil, errors.New("message header has invalid size")
	}
	restBuf := make([]byte, header.Size-4)

	for read := 0; int32(read) < header.Size-4; {
		n, err := reader.Read(restBuf[read:])
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if n == 0 {
			break
		}
		read += n
	}

	if len(restBuf) < 12 {
		return nil, errors.Errorf("invalid message header. either header.Size = %v is shorter than message length, or message is missing RequestId, ResponseTo, or OpCode fields.", header.Size)
	}
	header.RequestID = readInt32(restBuf)
	header.ResponseTo = readInt32(restBuf[4:])
	header.OpCode = OpType(readInt32(restBuf[8:]))

	return header.Parse(restBuf[12:])
}

func SendMessage(m Message, writer io.Writer) error {
	buf := m.Serialize()

	for {
		written, err := writer.Write(buf)
		if err != nil {
			return errors.Wrap(err, "error writing to client")
		}

		if written == len(buf) {
			return nil
		}

		buf = buf[written:]
	}
}
