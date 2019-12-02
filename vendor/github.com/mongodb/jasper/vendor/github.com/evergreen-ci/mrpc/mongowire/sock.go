package mongowire

import (
	"context"
	"io"

	"github.com/pkg/errors"
)

const MaxInt32 = 2147483647

func ReadMessage(ctx context.Context, reader io.Reader) (Message, error) {
	type readResult struct {
		n   int
		err error
	}

	// read header
	sizeBuf := make([]byte, 4)

	readFinished := make(chan readResult)
	go func() {
		defer close(readFinished)
		n, err := reader.Read(sizeBuf)
		select {
		case readFinished <- readResult{n: n, err: err}:
		case <-ctx.Done():
		}
	}()
	select {
	case <-ctx.Done():
		return nil, errors.WithStack(ctx.Err())
	case res := <-readFinished:
		if res.err != nil {
			return nil, errors.WithStack(res.err)
		}
		if res.n != 4 {
			return nil, errors.Errorf("didn't read message size from socket, got %d", res.n)
		}
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
		readFinished = make(chan readResult)
		go func() {
			defer close(readFinished)
			n, err := reader.Read(restBuf)
			select {
			case readFinished <- readResult{n: n, err: err}:
			case <-ctx.Done():
			}
		}()
		select {
		case <-ctx.Done():
			return nil, errors.WithStack(ctx.Err())
		case res := <-readFinished:
			if res.err != nil {
				return nil, errors.WithStack(res.err)
			}
			if res.n == 0 {
				break
			}
			read += res.n
		}
	}

	if len(restBuf) < 12 {
		return nil, errors.Errorf("invalid message header. either header.Size = %v is shorter than message length, or message is missing RequestId, ResponseTo, or OpCode fields.", header.Size)
	}
	header.RequestID = readInt32(restBuf)
	header.ResponseTo = readInt32(restBuf[4:])
	header.OpCode = OpType(readInt32(restBuf[8:]))

	return header.Parse(restBuf[12:])
}

func SendMessage(ctx context.Context, m Message, writer io.Writer) error {
	buf := m.Serialize()

	type writeRes struct {
		n   int
		err error
	}
	for {
		writeFinished := make(chan writeRes)
		go func() {
			defer close(writeFinished)
			n, err := writer.Write(buf)
			select {
			case writeFinished <- writeRes{n: n, err: err}:
			case <-ctx.Done():
			}
		}()
		select {
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		case res := <-writeFinished:
			if res.err != nil {
				return errors.Wrap(res.err, "error writing message to client")
			}
			if res.n == len(buf) {
				return nil
			}
			buf = buf[res.n:]
		}
	}
}
