// +build go1.7

package rest

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type QueueClientSuite struct {
	service *QueueService
	client  *QueueClient
	server  *httptest.Server
	info    struct {
		host string
		port int
	}
	require *require.Assertions
	closer  context.CancelFunc
	suite.Suite
}

func TestQueueClientSuite(t *testing.T) {
	suite.Run(t, new(QueueClientSuite))
}

func (s *QueueClientSuite) SetupSuite() {
	job.RegisterDefaultJobs()

	s.require = s.Require()
	ctx, cancel := context.WithCancel(context.Background())
	s.closer = cancel
	s.service = NewQueueService()
	s.NoError(s.service.Open(ctx))

	app := s.service.App()
	s.NoError(app.Resolve())
	router, err := s.service.App().Handler()
	s.NoError(err)

	s.server = httptest.NewServer(router)

	portStart := strings.LastIndex(s.server.URL, ":")
	port, err := strconv.Atoi(s.server.URL[portStart+1:])
	s.require.NoError(err)
	s.info.host = s.server.URL[:portStart]
	s.info.port = port
	grip.Infof("running test rest service at '%s', on port '%d'", s.info.host, s.info.port)
}

func (s *QueueClientSuite) TearDownSuite() {
	grip.Infof("closing test rest service at '%s', on port '%d'", s.info.host, s.info.port)
	s.server.Close()
	s.closer()
}

func (s *QueueClientSuite) SetupTest() {
	s.client = &QueueClient{}
}

////////////////////////////////////////////////////////////////////////
//
// A collection of tests that exercise and test the consistency and
// validation in the configuration interface for the rest client.
//
////////////////////////////////////////////////////////////////////////

func (s *QueueClientSuite) TestSetHostRequiresHttpURL() {
	example := "http://exmaple.com"

	s.Equal("", s.client.Host())
	s.NoError(s.client.SetHost(example))
	s.Equal(example, s.client.Host())

	badURI := []string{"foo", "1", "true", "htp", "ssh"}

	for _, uri := range badURI {
		s.Error(s.client.SetHost(uri))
		s.Equal(example, s.client.Host())
	}
}

func (s *QueueClientSuite) TestSetHostStripsTrailingSlash() {
	uris := []string{
		"http://foo.example.com/",
		"https://extra.example.net/bar/s/",
	}

	for _, uri := range uris {
		s.True(strings.HasSuffix(uri, "/"))
		s.NoError(s.client.SetHost(uri))
		s.Equal(uri[:len(uri)-1], s.client.Host())
		s.False(strings.HasSuffix(s.client.Host(), "/"))
	}
}

func (s *QueueClientSuite) TestSetHostRoundTripsValidHostWithGetter() {
	uris := []string{
		"http://foo.example.com",
		"https://extra.example.net/bar/s",
	}
	for _, uri := range uris {
		s.NoError(s.client.SetHost(uri))
		s.Equal(uri, s.client.Host())
	}
}

func (s *QueueClientSuite) TestPortSetterDisallowsPortsToBeZero() {
	s.Equal(0, s.client.port)
	s.Equal(0, s.client.Port())

	s.Error(s.client.SetPort(0))
	s.Equal(3000, s.client.Port())
}

func (s *QueueClientSuite) TestPortSetterDisallowsTooBigPorts() {
	s.Equal(0, s.client.port)
	s.Equal(0, s.client.Port())

	for _, p := range []int{65536, 70000, 1000000} {
		s.Error(s.client.SetPort(p), strconv.Itoa(p))
		s.Equal(3000, s.client.Port())
	}
}

func (s *QueueClientSuite) TestPortSetterRoundTripsValidPortsWithGetter() {
	for _, p := range []int{65, 8080, 1400} {
		s.NoError(s.client.SetPort(p), strconv.Itoa(p))
		s.Equal(p, s.client.Port())
	}
}

func (s *QueueClientSuite) TestSetPrefixRemovesTrailingAndLeadingSlashes() {
	s.Equal("", s.client.Prefix())

	for _, p := range []string{"/foo", "foo/", "/foo/"} {
		s.NoError(s.client.SetPrefix(p))
		s.Equal("foo", s.client.Prefix())
	}
}

func (s *QueueClientSuite) TestSetPrefixRoundTripsThroughGetter() {
	for _, p := range []string{"", "foo/bar", "foo", "foo/bar/baz"} {
		s.NoError(s.client.SetPrefix(p))
		s.Equal(p, s.client.Prefix())
	}
}

////////////////////////////////////////////////////////////////////////
//
// Client Initialization Checks/Tests
//
////////////////////////////////////////////////////////////////////////

func (s *QueueClientSuite) TestNewQueueClientPropogatesValidValuesToCreatedValues() {
	nc, err := NewQueueClient("http://example.com", 8080, "amboy_test")
	s.NoError(err)

	s.Equal(8080, nc.Port())
	s.Equal("http://example.com", nc.Host())
	s.Equal("amboy_test", nc.Prefix())
}

func (s *QueueClientSuite) TestCorrectedNewQueueClientSettings() {
	nc, err := NewQueueClient("http://example.com", 900000000, "/amboy/")
	s.Error(err)
	s.Nil(nc)
}

func (s *QueueClientSuite) TestNewQueueClientConstructorPropogatesErrorStateForHost() {
	nc, err := NewQueueClient("foo", 3000, "")

	s.Nil(nc)
	s.Error(err)
}

func (s *QueueClientSuite) TestNewQueueClientFromExistingWithNilClientReturnsError() {
	nc, err := NewQueueClientFromExisting(nil, "http://example.com", 2048, "amboy_test")
	s.Error(err)
	s.Nil(nc)
}

////////////////////////////////////////////////////////////////////////
//
// Client/Service Interaction: internals and helpers
//
////////////////////////////////////////////////////////////////////////

func (s *QueueClientSuite) TestURLGeneratiorWithoutDefaultPortInResult() {
	s.NoError(s.client.SetHost("http://amboy.example.net"))

	for _, p := range []int{0, 80} {
		s.client.port = p

		s.Equal("http://amboy.example.net/foo", s.client.getURL("foo"))
	}
}

func (s *QueueClientSuite) TestURLGenerationWithNonDefaultPort() {
	for _, p := range []int{82, 8080, 3000, 42420, 2048} {
		s.NoError(s.client.SetPort(p))
		host := "http://amboy.example.net"
		s.NoError(s.client.SetHost(host))
		prefix := "/queue"
		s.NoError(s.client.SetPrefix(prefix))
		endpoint := "/status"
		expected := strings.Join([]string{host, ":", strconv.Itoa(p), prefix, endpoint}, "")

		s.Equal(expected, s.client.getURL(endpoint))
	}
}

func (s *QueueClientSuite) TestURLGenerationWithEmptyPrefix() {
	host := "http://amboy.example.net"
	endpoint := "status"

	s.NoError(s.client.SetHost(host))
	s.Equal("", s.client.Prefix())

	s.Equal(strings.Join([]string{host, endpoint}, "/"),
		s.client.getURL(endpoint))
}

func (s *QueueClientSuite) TestGetStatusOperationWithoutRunningServerReturnsError() {
	var err error

	s.client, err = NewQueueClient("http://example.net", 3000, "/amboy")
	s.require.NoError(err)
	s.client.client.Timeout = 5 * time.Millisecond
	ctx := context.Background()

	st, err := s.client.getStats(ctx)
	s.Error(err)
	s.Nil(st)
}

func (s *QueueClientSuite) TestGetStatusWithServerAndCanceled() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	cancel()

	st, err := s.client.getStats(ctx)
	s.Error(err)
	s.Nil(st)
}

func (s *QueueClientSuite) TestGetStatusResponseHasExpectedValues() {
	var err error
	ctx := context.Background()
	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	st, err := s.client.getStats(ctx)
	s.NoError(err)
	s.True(st.QueueRunning)
	s.Equal("ok", st.Status)
	s.Equal(0, st.PendingJobs)
	s.Len(st.SupportedJobTypes, 2, fmt.Sprint(st.SupportedJobTypes))
}

func (s *QueueClientSuite) TestGetStatsHelperWithInvalidHostReturnsError() {
	var err error
	ctx := context.Background()
	s.client, err = NewQueueClient(s.info.host+".1", s.info.port, "")
	s.client.client.Timeout = 5 * time.Millisecond
	s.NoError(err)

	st, err := s.client.getStats(ctx)
	s.Nil(st)
	s.Error(err)
}

func (s *QueueClientSuite) TestGetStatsHelperWithCanceledContextReturnsError() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	cancel()
	st, err := s.client.getStats(ctx)
	s.Nil(st)
	s.Error(err)

}

func (s *QueueClientSuite) TestGetStatsHelperWithActualJob() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	j := job.NewShellJob("true", "")

	s.NoError(s.service.queue.Put(ctx, j))
	amboy.Wait(ctx, s.service.queue)

	st, err := s.client.getStats(ctx)
	s.NoError(err, fmt.Sprintf("%+v", st))
}

func (s *QueueClientSuite) TestJobStatusWithCanceledContextReturnsError() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	cancel()

	st, err := s.client.jobStatus(ctx, "foo")
	s.Nil(st)
	s.Error(err)
}

func (s *QueueClientSuite) TestJobStatusWithInvalidURLReturnsError() {
	var err error
	ctx := context.Background()
	s.client, err = NewQueueClient(s.info.host+".1", s.info.port, "")
	s.client.client.Timeout = 5 * time.Millisecond
	s.NoError(err)

	st, err := s.client.jobStatus(ctx, "foo")
	s.Error(err)
	s.Nil(st)
}

func (s *QueueClientSuite) TestJobStatusWithNonExistingJobReturnsError() {
	var err error
	ctx := context.Background()
	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	st, err := s.client.jobStatus(ctx, "foo")
	s.NoError(err)

	s.False(st.Exists)
	s.False(st.Completed)
	s.Equal("foo", st.ID)
}

func (s *QueueClientSuite) TestJobStatusWithValidJob() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	j := job.NewShellJob("echo foo", "")
	s.NoError(s.service.queue.Put(ctx, j))
	amboy.Wait(ctx, s.service.queue)
	st, err := s.client.jobStatus(ctx, j.ID())
	s.NoError(err)
	s.Equal(j.ID(), st.ID)
	s.True(st.Exists)
	s.True(st.Completed)
	s.Equal(0, st.JobsPending)
}

////////////////////////////////////////////////////////////////////////
//
// Public Client Interfaces
//
////////////////////////////////////////////////////////////////////////

func (s *QueueClientSuite) TestRunningMethodWithCanceledContextReturnsError() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	running, err := s.client.Running(ctx)
	s.Error(err)
	s.False(running)
}

func (s *QueueClientSuite) TestRunningMethodWithoutRunningQueue() {
	var err error
	existing := s.service.queue
	s.service.queue = nil
	ctx := context.Background()

	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	running, err := s.client.Running(ctx)
	s.NoError(err)
	s.False(running)

	s.service.queue = existing
}

func (s *QueueClientSuite) TestRunningWiwthRunningQueue() {
	var err error
	ctx := context.Background()
	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	s.True(s.service.queue.Started())

	running, err := s.client.Running(ctx)
	s.NoError(err)
	s.True(running)
}

func (s *QueueClientSuite) TestPendingJobsWithCanceledContextReturnsError() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	cancel()

	pending, err := s.client.PendingJobs(ctx)

	s.Equal(-1, pending)
	s.Error(err)
}

func (s *QueueClientSuite) TestPendingJobsIsZeroAfterWaitingOnTheQueue() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	amboy.Wait(ctx, s.service.queue)
	var err error

	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)
	pending, err := s.client.PendingJobs(ctx)

	s.Equal(0, pending)
	s.NoError(err)
}

func (s *QueueClientSuite) TestJobCompleteWithCanceledContextReturnsError() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	cancel()

	isComplete, err := s.client.JobComplete(ctx, "foo")

	s.False(isComplete)
	s.Error(err)
}

func (s *QueueClientSuite) TestJobCompleteIsZeroAfterWaitingOnTheQueue() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	amboy.Wait(ctx, s.service.queue)
	var err error

	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	s.NoError(err)

	j := job.NewShellJob("echo foo", "")
	s.NoError(s.service.queue.Put(ctx, j))

	amboy.Wait(ctx, s.service.queue)

	isComplete, err := s.client.JobComplete(ctx, j.ID())
	s.True(isComplete)
	s.NoError(err)
}

func (s *QueueClientSuite) TestJobSubmitWithCanceledContextReturnsError() {
	var err error
	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	ctx, cancel := context.WithCancel(context.Background())
	s.NoError(err)

	cancel()
	j := job.NewShellJob("echo what", "")
	name, err := s.client.SubmitJob(ctx, j)

	s.Equal("", name)
	s.Error(err)
}

func (s *QueueClientSuite) TestSubmitJobReturnsSuccessfulJobId() {
	var err error
	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	ctx := context.Background()
	s.NoError(err)

	j := job.NewShellJob("echo foo", "")
	name, err := s.client.SubmitJob(ctx, j)
	s.Equal(j.ID(), name)
	s.NoError(err)
}

func (s *QueueClientSuite) TestSubmitDuplicateJobReturnsError() {
	var err error
	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	ctx := context.Background()
	s.NoError(err)

	j := job.NewShellJob("echo foo", "")

	// first time works
	name, err := s.client.SubmitJob(ctx, j)
	s.NoError(err)
	s.Equal(j.ID(), name)

	// second time doesn't
	name, err = s.client.SubmitJob(ctx, j)
	s.Error(err)
	s.Equal("", name)
}

func (s *QueueClientSuite) TestWhenWaitMethodReturnsJobsAreComplete() {
	var err error
	var name string

	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	ctx := context.Background()
	s.NoError(err)

	for i := 0; i < 10; i++ {
		j := job.NewShellJob(fmt.Sprintf("echo %d", i), "")
		name, err = s.client.SubmitJob(ctx, j)
		s.Equal(j.ID(), name)
		s.NoError(err)

		ok := s.client.Wait(ctx, name)
		s.True(ok)
	}
}

func (s *QueueClientSuite) TestWhenWaitAllMethodReturnsAllJobsAreComplete() {
	var err error

	s.client, err = NewQueueClient(s.info.host, s.info.port, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(err)

	for i := 0; i < 100; i++ {
		err = s.service.queue.Put(ctx, job.NewShellJob(fmt.Sprintf("echo %d", i), ""))
		s.NoError(err)
	}

	qst := s.service.queue.Stats(ctx)
	s.NotEqual(0, qst.Pending)

	ok := s.client.WaitAll(ctx)
	s.True(ok)

	qst = s.service.queue.Stats(ctx)
	s.Equal(0, qst.Pending)
}

func (s *QueueClientSuite) TestFetchJobReturnsEquivalentJob() {
	var err error

	s.client, err = NewQueueClient(s.info.host, s.info.port, "")
	ctx := context.Background()
	s.NoError(err)
	jobs := []*job.ShellJob{}

	for i := 0; i < 10; i++ {
		j := job.NewShellJob(fmt.Sprintf("echo %d", i), "")
		_, err = s.client.SubmitJob(ctx, j)
		s.NoError(err)
		jobs = append(jobs, j)
	}

	ok := s.client.WaitAll(ctx)
	s.True(ok)

	for _, j := range jobs {
		rj, err := s.client.FetchJob(ctx, j.ID())
		if s.NoError(err) {
			s.Equal(j.ID(), rj.ID())
			s.Equal(j.Command, rj.(*job.ShellJob).Command)
		}
	}
}

// TODO: wait
// TODO: waitAll
