package grip

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tychoish/grip/send"
)

type GripSuite struct {
	grip Journaler
	name string
	suite.Suite
}

func TestGripSuite(t *testing.T) {
	suite.Run(t, new(GripSuite))
}

func (s *GripSuite) SetupSuite() {
	s.grip = NewJournaler(s.name)
	s.Equal(s.grip.Name(), s.name)
}

func (s *GripSuite) SetupTest() {
	s.grip.SetName(s.name)
	s.NoError(s.grip.SetSender(send.NewBootstrapLogger(s.name, s.grip.GetSender().Level())))
}

func (s *GripSuite) TestDefaultJournalerIsBootstrap() {
	s.Equal(s.grip.GetSender().Type(), send.Bootstrap)

	// the bootstrap sender is a bit special because you can't
	// change it's name, therefore:
	secondName := "something_else"
	s.grip.SetName(secondName)

	s.Equal(s.grip.GetSender().Type(), send.Bootstrap)
	s.Equal(s.grip.Name(), secondName)
}

func (s *GripSuite) TestNameSetterAndGetter() {
	for _, name := range []string{"a", "a39df", "a@)(*E)"} {
		s.grip.SetName(name)
		s.Equal(s.grip.Name(), name)
	}
}
