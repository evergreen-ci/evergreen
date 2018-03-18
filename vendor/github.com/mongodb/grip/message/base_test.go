package message

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ComposerBaseSuite struct {
	base *Base
	suite.Suite
}

func TestComposerBaseSuite(t *testing.T) {
	suite.Run(t, new(ComposerBaseSuite))
}

func (s *ComposerBaseSuite) SetupTest() {
	s.base = &Base{}
}

func (s *ComposerBaseSuite) TestCollectWorksWithUnsetPids() {
	s.Equal("", s.base.Hostname)
	s.base.Pid = 0
	s.NoError(s.base.Collect())
	s.NotEqual("", s.base.Hostname)

}

func (s *ComposerBaseSuite) TestCollectNoopsIfPidIsSet() {
	s.Equal("", s.base.Hostname)
	s.base.Pid = 1
	s.NoError(s.base.Collect())
	s.Equal("", s.base.Hostname)
}

func (s *ComposerBaseSuite) TestAnnotateAddsFields() {
	s.Nil(s.base.Context)
	s.NoError(s.base.Annotate("k", "foo"))
	s.NotNil(s.base.Context)
}

func (s *ComposerBaseSuite) TestAnnotateErrorsForSameValue() {
	s.NoError(s.base.Annotate("k", "foo"))
	s.Error(s.base.Annotate("k", "foo"))

	s.Equal("foo", s.base.Context["k"])
}

func (s *ComposerBaseSuite) TestAnnotateMultipleValues() {
	s.NoError(s.base.Annotate("kOne", "foo"))
	s.NoError(s.base.Annotate("kTwo", "foo"))
	s.Equal("foo", s.base.Context["kOne"])
	s.Equal("foo", s.base.Context["kTwo"])
}
