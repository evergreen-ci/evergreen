package operations

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
)

type PatchUtilTestSuite struct {
	suite.Suite
	tempDir        string
	testConfigFile string
}

func TestPatchUtilTestSuite(t *testing.T) {
	suite.Run(t, new(PatchUtilTestSuite))
}

func (s *PatchUtilTestSuite) SetupSuite() {
	dir, err := ioutil.TempDir("", "")
	s.Require().NoError(err)

	s.tempDir = dir
	s.testConfigFile = dir + ".evergreen.yml"
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

	err := ioutil.WriteFile(s.testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{Project: "mci"}
	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	s.Require().NoError(pp.loadAlias(conf))
	s.Require().NoError(pp.loadVariants(conf))
	s.Require().NoError(pp.loadTasks(conf))

	s.Equal("testing", pp.Alias)
	s.Nil(pp.Variants)
	s.Nil(pp.Tasks)
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

	err := ioutil.WriteFile(s.testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{Project: "mci"}
	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	s.Require().NoError(pp.loadAlias(conf))
	s.Require().NoError(pp.loadVariants(conf))
	s.Require().NoError(pp.loadTasks(conf))

	s.Zero(pp.Alias)
	s.Contains(pp.Variants, "myvariant1")
	s.Contains(pp.Variants, "myvariant2")
	s.Contains(pp.Tasks, "mytask1")
	s.Contains(pp.Tasks, "mytask2")
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

	err := ioutil.WriteFile(s.testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{
		Project:     "mci",
		Alias:       "testing",
		SkipConfirm: true,
	}
	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	s.Require().NoError(pp.loadAlias(conf))
	s.Require().NoError(pp.loadVariants(conf))
	s.Require().NoError(pp.loadTasks(conf))

	s.Equal("testing", pp.Alias)
	s.Nil(pp.Variants)
	s.Nil(pp.Tasks)
}

func (s *PatchUtilTestSuite) TestVariantsTasksFromCLI() {
	// Set up the user config file
	fileContents := `projects:
- name: mci
  default: true
  alias: testing`

	err := ioutil.WriteFile(s.testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{
		Project:     "mci",
		Variants:    []string{"myvariant1", "myvariant2"},
		Tasks:       []string{"mytask1", "mytask2"},
		SkipConfirm: true,
	}
	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	s.Require().NoError(pp.loadAlias(conf))
	s.Require().NoError(pp.loadVariants(conf))
	s.Require().NoError(pp.loadTasks(conf))

	s.Zero(pp.Alias)
	s.Contains(pp.Variants, "myvariant1")
	s.Contains(pp.Variants, "myvariant2")
	s.Contains(pp.Tasks, "mytask1")
	s.Contains(pp.Tasks, "mytask2")
}

func (s *PatchUtilTestSuite) TestAddMetadataToDiff() {
	metadata := GitMetadata{
		Username:    "octocat",
		Email:       "octocat@github.com",
		CurrentTime: "Tue, 7 Jul 2020 16:50:42 -0400",
		GitVersion:  "2.19.1",
		Subject:     "EVG-12345 diff to mbox",
	}

	diffData := &localDiff{
		fullPatch: "+ func diffToMbox(diffData *localDiff, subject string) (string, error) {",
		log:       "operations/patch_util.go           |  17 ---",
	}

	mboxDiff, err := addMetadataToDiff(diffData, metadata)
	s.NoError(err)
	s.Equal(`From 72899681697bc4c45b1dae2c97c62e2e7e5d597b Mon Sep 17 00:00:00 2001
From: octocat <octocat@github.com>
Date: Tue, 7 Jul 2020 16:50:42 -0400
Subject: EVG-12345 diff to mbox

---
operations/patch_util.go           |  17 ---

+ func diffToMbox(diffData *localDiff, subject string) (string, error) {
--
2.19.1
`, mboxDiff)
}

func (s *PatchUtilTestSuite) TestParseGitVersionString() {
	versionStrings := map[string]string{
		"git version 2.19.1":                   "2.19.1",
		"git version 2.24.3 (Apple Git-128)":   "2.24.3",
		"git version 2.21.1 (Apple Git-122.3)": "2.21.1",
		"git version 2.16.2.windows.1":         "2.16.2.windows.1",
	}

	for versionString, version := range versionStrings {
		parsedVersion, err := parseGitVersion(versionString)
		s.NoError(err)
		s.Equal(version, parsedVersion)
	}
}

func (s *PatchUtilTestSuite) TearDownSuite() {
	s.Require().NoError(os.RemoveAll(s.tempDir))
}
