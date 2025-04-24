package plugin

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/suite"
)

func TestFileVisibility(t *testing.T) {
	s := &TestFileVisibilitySuite{}
	suite.Run(t, s)
}

type TestFileVisibilitySuite struct {
	files []artifact.File
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *TestFileVisibilitySuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.files = []artifact.File{
		{Name: "Private", Visibility: artifact.Private},
		{Name: "Public", Visibility: artifact.Public},
		{Name: "Hidden", Visibility: artifact.None},
		{Name: "Unset", Visibility: ""},
		{Name: "Signed", Visibility: artifact.Signed, AWSKey: "key", AWSSecret: "secret", Bucket: "bucket", FileKey: "filekey"},
	}
}

func (s *TestFileVisibilitySuite) TearDownTest() {
	s.cancel()
}

func (s *TestFileVisibilitySuite) TestFileVisibilityWithoutUser() {
	stripped, err := artifact.StripHiddenFiles(s.ctx, s.files, false)
	s.Require().NoError(err)
	s.Len(s.files, 5)

	s.Equal("Public", stripped[0].Name)
	s.Equal("Unset", stripped[1].Name)
	s.Len(stripped, 2)
}

func (s *TestFileVisibilitySuite) TestFileVisibilityWithUser() {
	hasUser := &user.DBUser{} != nil
	stripped, err := artifact.StripHiddenFiles(s.ctx, s.files, hasUser)
	s.Require().NoError(err)
	s.Len(s.files, 5)

	s.Equal("Private", stripped[0].Name)
	s.Equal("Public", stripped[1].Name)
	s.Equal("Unset", stripped[2].Name)
	s.Equal("Signed", stripped[3].Name)
	s.Len(stripped, 4)
}
