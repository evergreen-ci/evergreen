package operations

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
)

const testConfigFile = ".evergreen.yml"

type PatchUtilTestSuite struct {
	suite.Suite
	tempDir string
}

func TestPatchUtilTestSuite(t *testing.T) {
	suite.Run(t, new(PatchUtilTestSuite))
}

func (s *PatchUtilTestSuite) SetupSuite() {
	dir, err := ioutil.TempDir("", "")
	s.Require().NoError(err)

	s.tempDir = dir
}

func (s *PatchUtilTestSuite) TestLoadAliasFromFile() {
	// Set up the user config file
	fileContents := `projects:
- name: mci
  default: true
  alias: testing
  variants:
   - myvariant1
   - myvariant2
  tasks:
   - mytask1
   - mytask2`

	err := ioutil.WriteFile(s.tempDir+testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{Project: "mci"}
	conf, err := NewClientSettings(s.tempDir + testConfigFile)

	pp.loadAlias(conf)
	pp.loadVariants(conf)
	pp.loadTasks(conf)

	s.Equal("testing", pp.Alias)
	s.Equal([]string(nil), pp.Variants)
	s.Equal([]string(nil), pp.Tasks)
}

func (s *PatchUtilTestSuite) TestLoadVariantsTasksFromFile() {
	// Set up the user config file
	fileContents := `projects:
- name: mci
  default: true
  variants:
   - myvariant1
   - myvariant2
  tasks:
   - mytask1
   - mytask2`

	err := ioutil.WriteFile(s.tempDir+testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{Project: "mci"}
	conf, err := NewClientSettings(s.tempDir + testConfigFile)

	pp.loadAlias(conf)
	pp.loadVariants(conf)
	pp.loadTasks(conf)

	s.Equal("", pp.Alias)
	s.Equal([]string{"myvariant1", "myvariant2"}, pp.Variants)
	s.Equal([]string{"mytask1", "mytask2"}, pp.Tasks)
}

func (s *PatchUtilTestSuite) TestAliasFromCLI() {
	// Set up the user config file
	fileContents := `projects:
- name: mci
  default: true
  variants:
   - myvariant1
   - myvariant2
  tasks:
   - mytask1
   - mytask2`

	err := ioutil.WriteFile(s.tempDir+testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{
		Project:     "mci",
		Alias:       "testing",
		SkipConfirm: true,
	}
	conf, err := NewClientSettings(s.tempDir + testConfigFile)

	pp.loadAlias(conf)
	pp.loadVariants(conf)
	pp.loadTasks(conf)

	s.Equal("testing", pp.Alias)
	s.Equal([]string(nil), pp.Variants)
	s.Equal([]string(nil), pp.Tasks)
}

func (s *PatchUtilTestSuite) TestVariantsTasksFromCLI() {
	// Set up the user config file
	fileContents := `projects:
- name: mci
  default: true
  alias: testing`

	err := ioutil.WriteFile(s.tempDir+testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{
		Project:     "mci",
		Variants:    []string{"myvariant1", "myvariant2"},
		Tasks:       []string{"mytask1", "mytask2"},
		SkipConfirm: true,
	}
	conf, err := NewClientSettings(s.tempDir + testConfigFile)

	pp.loadAlias(conf)
	pp.loadVariants(conf)
	pp.loadTasks(conf)

	s.Equal("", pp.Alias)
	s.Equal([]string{"myvariant1", "myvariant2"}, pp.Variants)
	s.Equal([]string{"mytask1", "mytask2"}, pp.Tasks)
}

func (s *PatchUtilTestSuite) TearDownSuite() {
	os.RemoveAll(s.tempDir)
}
