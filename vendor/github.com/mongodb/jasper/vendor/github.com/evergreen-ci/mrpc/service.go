package mrpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"

	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

type HandlerFunc func(context.Context, io.Writer, mongowire.Message)

type Service interface {
	Address() string
	RegisterOperation(scope *mongowire.OpScope, h HandlerFunc) error
	Run(context.Context) error
}

type basicService struct {
	addr     string
	registry *OperationRegistry
}

// NewService starts a generic wire protocol service listening on the given host
// and port.
func NewBasicService(host string, port int) Service {
	return &basicService{
		addr:     fmt.Sprintf("%s:%d", host, port),
		registry: &OperationRegistry{ops: make(map[mongowire.OpScope]HandlerFunc)},
	}
}

func (s *basicService) Address() string { return s.addr }

func (s *basicService) RegisterOperation(scope *mongowire.OpScope, h HandlerFunc) error {
	return errors.WithStack(s.registry.Add(*scope, h))
}

func (s *basicService) Run(ctx context.Context) error {
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

func (s *basicService) dispatchRequest(ctx context.Context, conn net.Conn) {
	defer func() {
		err := recovery.HandlePanicWithError(recover(), nil, "connection handling")
		grip.Error(message.WrapError(err, "error during request handling"))
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
