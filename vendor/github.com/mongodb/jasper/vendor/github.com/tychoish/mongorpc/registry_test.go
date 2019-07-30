package mongorpc

import (
	"fmt"
	"io"
	"testing"

	"github.com/mongodb/grip"
	"github.com/stretchr/testify/suite"
	"github.com/tychoish/mongorpc/mongowire"
	"golang.org/x/net/context"
)

type RegistrySuite struct {
	registry *OperationRegistry
	handler  HandlerFunc
	suite.Suite
}

func TestRegistrySuite(t *testing.T) {
	suite.Run(t, new(RegistrySuite))
}

func (s *RegistrySuite) SetupSuite() {
	var callCount int
	s.handler = func(ctx context.Context, w io.Writer, m mongowire.Message) {
		callCount++
		grip.Infof("test handler, call %d", callCount)
	}
}

func (s *RegistrySuite) SetupTest() {
	s.registry = &OperationRegistry{
		ops: map[mongowire.OpScope]HandlerFunc{},
	}
}

func (s *RegistrySuite) TestOperationCanOnlyBeRegisteredOnce() {
	op := mongowire.OpScope{
		Type:    mongowire.OP_COMMAND,
		Context: "foo",
		Command: "bar",
	}

	s.Len(s.registry.ops, 0)
	s.NoError(s.registry.Add(op, s.handler))
	s.Len(s.registry.ops, 1)
	for i := 0; i < 100; i++ {
		s.Error(s.registry.Add(op, s.handler))
		s.Len(s.registry.ops, 1)
	}

	// test with the same content, even if it's a different object
	op2 := mongowire.OpScope{
		Type:    mongowire.OP_COMMAND,
		Context: "foo",
		Command: "bar",
	}

	s.Error(s.registry.Add(op2, s.handler))
	s.Len(s.registry.ops, 1)

	// add something new and make sure that it is added
	op3 := mongowire.OpScope{
		Type:    mongowire.OP_COMMAND,
		Context: "bar",
		Command: "bar",
	}
	s.NoError(s.registry.Add(op3, s.handler))
	s.Len(s.registry.ops, 2)
}

func (s *RegistrySuite) TestInvalidScopeIsNotAdded() {
	op := mongowire.OpScope{}

	s.Error(s.registry.Add(op, s.handler))
	op.Type = mongowire.OP_KILL_CURSORS
	op.Context = "foo"
	s.Error(s.registry.Add(op, s.handler))
	s.Len(s.registry.ops, 0)
}

func (s *RegistrySuite) TestOpsMustHaveValidHandlers() {
	op := mongowire.OpScope{
		Type:    mongowire.OP_COMMAND,
		Context: "foo",
		Command: "bar",
	}

	s.Len(s.registry.ops, 0)
	s.NoError(op.Validate())

	s.Error(s.registry.Add(op, nil))
	s.Len(s.registry.ops, 0)
}

func (s *RegistrySuite) TestUndefinedOperationsRetreiveNilResults() {
	op := mongowire.OpScope{}

	s.Len(s.registry.ops, 0)
	h, ok := s.registry.Get(&op)
	s.False(ok)
	s.Nil(h)
}

func (s *RegistrySuite) TestOpsAreRetreivable() {
	op := mongowire.OpScope{
		Type:    mongowire.OP_COMMAND,
		Context: "foo",
		Command: "bar",
	}

	s.NoError(s.registry.Add(op, s.handler))
	s.Len(s.registry.ops, 1)
	h, ok := s.registry.Get(&op)
	s.True(ok)
	s.NotNil(h)
	s.Equal(fmt.Sprint(h), fmt.Sprint(s.handler))
}
