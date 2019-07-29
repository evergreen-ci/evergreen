package reliability

import (
        "testing"

        _ "github.com/evergreen-ci/evergreen/testutil"
        "github.com/stretchr/testify/suite"
)

type taskReliabilityQuerySuite struct {
        suite.Suite
}

func TestStatsQuerySuite(t *testing.T) {
        suite.Run(t, new(taskReliabilityQuerySuite))
}

func (s *taskReliabilityQuerySuite) SetupTest() {
}

func (s *taskReliabilityQuerySuite) TestValidFilter() {
        require := s.Require()

        project := "mongodb-mongo-master"

        filter := TaskReliabilityFilter{
                Project: project,
        }

        // Check that validate does not find any errors
        // For tests
        err := filter.ValidateForTaskReliability()
        require.NoError(err)
}
