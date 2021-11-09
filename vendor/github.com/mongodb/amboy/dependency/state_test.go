package dependency

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

// StatesSuites checks the States types, which are the constants that
// define if a task needs to run or can be a noop.
type StatesSuite struct {
	suite.Suite
}

func TestStatesSuite(t *testing.T) {
	suite.Run(t, new(StatesSuite))
}

func (s *StatesSuite) TestStatValidatorReturnsFalseForInvalidValues() {
	s.False(IsValidState(State(5)))
	s.False(IsValidState(State(-1)))
}

func (s *StatesSuite) TestStateValidatorReturnsTrueForValidValues() {
	for i := 0; i < 4; i++ {
		s.True(IsValidState(State(i)))
	}
}

func (s StatesSuite) TestStringerInterfaceSatisfied() {
	s.Implements((*fmt.Stringer)(nil), State(0))
}

func (s *StatesSuite) TestStringerProducesValidStrings() {
	for i := 0; i < 4; i++ {
		state := State(i)

		s.False(strings.HasPrefix(state.String(), "%s"))
		s.False(strings.HasPrefix(state.String(), "State("))
	}

}

func (s *StatesSuite) TestStringerResturnsDefaultValueForOutOfBoundsStates() {
	s.True(strings.HasPrefix(State(-1).String(), "State("))
	s.True(strings.HasPrefix(State(5).String(), "State("))
}
