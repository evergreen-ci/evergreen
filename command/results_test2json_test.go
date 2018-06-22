package command

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

const (
	test2JSONFile = "command/testdata/test2json.json"
)

type test2JSONSuite struct {
	args map[string]interface{}
	c    *goTest2JSONCommand

	suite.Suite
}

func TestGoTest2JSON(t *testing.T) {
	suite.Run(t, &test2JSONSuite{})
}

func (s *test2JSONSuite) SetupTest() {
	s.c = goTest2JSONFactory().(*goTest2JSONCommand)
	s.c.Files = []string{test2JSONFile}

	s.args = map[string]interface{}{
		"files": []string{test2JSONFile},
	}
	s.Equal("gotest.parse_json", s.c.Name())
}

func (s *test2JSONSuite) TestNoFiles() {
	s.c.Files = []string{}
	s.args = map[string]interface{}{}
	s.EqualError(s.c.ParseParams(s.args), "error validating params: must specify at least one file pattern to parse: 'map[]'")

	s.c.Files = []string{}
	s.args = map[string]interface{}{
		"files": []string{},
	}
	s.EqualError(s.c.ParseParams(s.args), "error validating params: must specify at least one file pattern to parse: 'map[files:[]]'")
}

func (s *test2JSONSuite) TestParseArgs() {
	s.c.Files = []string{}
	s.args = map[string]interface{}{
		"files": []string{test2JSONFile, "some/other/file.json"},
	}
	s.NoError(s.c.ParseParams(s.args))
	s.Equal(s.args["files"], s.c.Files)
}
