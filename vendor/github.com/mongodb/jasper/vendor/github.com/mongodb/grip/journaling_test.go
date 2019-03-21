package grip

import (
	"testing"

	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
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
	sender, err := send.NewNativeLogger(s.name, s.grip.GetSender().Level())
	s.NoError(err)
	s.NoError(s.grip.SetSender(sender))
}

func (s *GripSuite) TestDefaultJournalerIsBootstrap() {
	firstName := s.grip.Name()
	// the bootstrap sender is a bit special because you can't
	// change it's name, therefore:
	secondName := "something_else"
	s.grip.SetName(secondName)

	s.Equal(s.grip.Name(), secondName)
	s.NotEqual(s.grip.Name(), firstName)
	s.NotEqual(firstName, secondName)
}

func (s *GripSuite) TestNameSetterAndGetter() {
	for _, name := range []string{"a", "a39df", "a@)(*E)"} {
		s.grip.SetName(name)
		s.Equal(s.grip.Name(), name)
	}
}
