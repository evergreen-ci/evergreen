package mrpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

type HandlerFunc func(context.Context, io.Writer, mongowire.Message)

type Service struct {
	addr     string
	registry *OperationRegistry
}

// NewService starts a service listening on the given address and port.
func NewService(listenAddr string, port int) *Service {
	return &Service{
		addr:     fmt.Sprintf("%s:%d", listenAddr, port),
		registry: &OperationRegistry{ops: make(map[mongowire.OpScope]HandlerFunc)},
	}
}

func (s *Service) Address() string { return s.addr }

func (s *Service) RegisterOperation(scope *mongowire.OpScope, h HandlerFunc) error {
	return errors.WithStack(s.registry.Add(*scope, h))
}

func (s *Service) Run(ctx context.Context) error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return errors.Wrapf(err, "problem listening on %s", s.addr)
	}
	defer l.Close()

	grip.Infof("listening for connections on %s", s.addr)

	for {
		if ctx.Err() != nil {
			return errors.New("service terminated by canceled context")
		}

		conn, err := l.Accept()
		if err != nil {
			grip.Warning(message.WrapError(err, "problem accepting connection"))
			continue
		}

		go s.dispatchRequest(ctx, conn)
	}
}

func writeErrorReply(w io.Writer, err error) error {
	responseNotOK := birch.EC.Int32("ok", 0)
	doc := birch.NewDocument(responseNotOK)
	if err != nil {
		doc.Append(birch.EC.String("errmsg", err.Error()))
	}
	// TODO: handle OP_MSG replies for newer protocol versions.
	reply := mongowire.NewReply(int64(0), int32(0), int32(0), int32(1), []birch.Document{*doc.Copy()})
	_, err = w.Write(reply.Serialize())
	return errors.Wrap(err, "could not write response")
}

func (s *Service) dispatchRequest(ctx context.Context, conn net.Conn) {
	defer func() {
		err := recovery.HandlePanicWithError(recover(), nil, "connection handling")
		if err != nil {
			grip.Error(message.WrapError(err, "error during request handling"))
			// Attempt to reply with the given deadline.
			if deadlineErr := conn.SetDeadline(time.Now().Add(15 * time.Second)); deadlineErr != nil {
				grip.Error(message.WrapError(err, "failed to set deadline on panic reply"))
			} else if writeErr := writeErrorReply(conn, err); writeErr != nil {
				grip.Error(message.WrapError(writeErr, "error writing reply after panic recovery"))
			}
		}
		if err := conn.Close(); err != nil {
			grip.Error(message.WrapErrorf(err, "error closing connection from %s", conn.RemoteAddr()))
			return
		}
		grip.Debugf("closed connection from %s", conn.RemoteAddr())
	}()

	if c, ok := conn.(*tls.Conn); ok {
		// we do this here so that we can get the SNI server name
		if err := c.Handshake(); err != nil {
			grip.Warning(message.WrapError(err, "error doing tls handshake"))
			return
		}
		grip.Debugf("ssl connection to %s", c.ConnectionState().ServerName)
	}

	for {
		m, err := mongowire.ReadMessage(ctx, conn)
		if err != nil {
			if errors.Cause(err) == io.EOF {
				// Connection was likely closed
				return
			}
			if ctx.Err() != nil {
				return
			}
			grip.Error(message.WrapError(err, "problem reading message"))
			return
		}

		scope := m.Scope()

		handler, ok := s.registry.Get(scope)
		if !ok {
			grip.Warningf("undefined command scope: %+v", scope)
			return
		}

		go handler(ctx, conn, m)
	}
}
