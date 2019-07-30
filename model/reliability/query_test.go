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
