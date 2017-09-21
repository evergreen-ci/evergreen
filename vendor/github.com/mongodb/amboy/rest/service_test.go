package rest

import (
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

type RestServiceSuite struct {
	service *Service
	require *require.Assertions
	suite.Suite
}

func TestRestServiceSuite(t *testing.T) {
	suite.Run(t, new(RestServiceSuite))
}

func (s *RestServiceSuite) SetupSuite() {
	// need to import job so that we load the init functions that
	// register jobs so these tests are more meaningful.
	job.RegisterDefaultJobs()
	s.require = s.Require()
}

func (s *RestServiceSuite) SetupTest() {
	s.service = NewService()
}

func (s *RestServiceSuite) TearDownTest() {
	s.service.Close()
}

func (s *RestServiceSuite) TestInitialListOfRegisteredJobs() {
	defaultJob := &Service{}
	s.Len(defaultJob.registeredTypes, 0)

	count := 0
	for _, jType := range s.service.registeredTypes {
		factory, err := registry.GetJobFactory(jType)
		if s.NoError(err) {
			count++
			s.Implements((*amboy.Job)(nil), factory())
		}
	}

	s.Len(s.service.registeredTypes, count)
}

func (s *RestServiceSuite) TestServiceOpenMethodInitializesResources() {
	s.Nil(s.service.closer)
	s.Nil(s.service.queue)

	ctx := context.Background()
	s.NoError(s.service.Open(ctx))

	s.NotNil(s.service.queue)
	s.NotNil(s.service.closer)
}

func (s *RestServiceSuite) TestIfNilAppMethodInitializesApp() {
	s.Nil(s.service.app)

	app := s.service.App()
	s.NotNil(app)
	s.Equal(app, s.service.app)
}
