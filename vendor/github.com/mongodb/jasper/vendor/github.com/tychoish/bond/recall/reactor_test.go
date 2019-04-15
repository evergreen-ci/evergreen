package recall

import (
	"errors"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/tychoish/bond"
)

type ReactorSuite struct {
	require *require.Assertions
	tempDir string
	suite.Suite
}

func TestReactorSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(ReactorSuite))
}

func (s *ReactorSuite) SetupSuite() {
	var err error
	s.require = s.Require()
	// s.tempDir, err = ioutil.TempDir("", uuid.NewV4().String())
	s.tempDir, err = ioutil.TempDir("", "")
	s.require.NoError(err)
}

func (s *ReactorSuite) TearDownSuite() {
	grip.Warningln("leaking tempdir for quicker tests:", s.tempDir)
	// err := os.RemoveAll(s.tempDir)
	// s.require.NoError(err)
}

func (s *ReactorSuite) TestJobCreator() {
	urls := make(chan string, 4)
	urls <- "https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-3.2.9.tgz"
	urls <- "https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-3.2.10.tgz"
	// we don't catch errors with invalid urls at this stage.
	urls <- "https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-2.8.9.tgz"
	urls <- "https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-2.8.10.tgz"
	close(urls)

	jobs, errs := createJobs(s.tempDir, urls)

	done := make(chan struct{})
	go func() {
		count := 0
		for range jobs {
			count++
		}
		s.Equal(count, 4)

		done <- struct{}{}
	}()
	<-done

	s.Nil(aggregateErrors(errs))
}

func (s *ReactorSuite) TestCreateJobsErrorsWithInvalidPath() {
	urls := make(chan string, 2)
	urls <- "https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-2.8.9.tgz"
	urls <- "https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-2.8.10.tgz"
	close(urls)
	fn := filepath.Join(s.tempDir, "foo")
	s.NoError(ioutil.WriteFile(fn, []byte("hello"), 0644))
	_, errs := createJobs(fn, urls)

	s.Error(aggregateErrors(errs))
}

func (s *ReactorSuite) TestDownloadValidReleases() {
	opts := bond.BuildOptions{
		Target:  "linux_x86_64",
		Arch:    "x86_64",
		Edition: "base",
		Debug:   false,
	}
	err := DownloadReleases([]string{"3.2.10"}, s.tempDir, opts)
	s.NoError(err)
}

func (s *ReactorSuite) TestDownloadInvalidReleasesAndOptions() {
	opts := bond.BuildOptions{
		Target:  "linux",
		Arch:    "amd64",
		Edition: "legacy",
		Debug:   false,
	}
	err := DownloadReleases([]string{"2.8.9", "2.8.10"}, s.tempDir, opts)
	s.Error(err)
}

func (s *ReactorSuite) TestDownloadValidOptionsWithInvalidReleases() {
	opts := bond.BuildOptions{
		Target:  "linux_x86_64",
		Arch:    "x86_64",
		Edition: "base",
		Debug:   false,
	}
	err := DownloadReleases([]string{"2.8.9", "2.8.10"}, s.tempDir, opts)
	s.Error(err)
}

func (s *ReactorSuite) TestDownloadInvalidOptionsWithValidReleases() {
	opts := bond.BuildOptions{
		Target: "linux",
	}

	err := DownloadReleases([]string{"3.2.9", "3.2.10"}, s.tempDir, opts)
	s.Error(err)
}

////////////////////////////////////////////////////////////////////////
//
// Stand Alone Tests
//
////////////////////////////////////////////////////////////////////////

func TestErrorAggregator(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// hand a single channel with no errors, and observe no aggregate errors.
	e := make(chan error)
	close(e)
	assert.NoError(aggregateErrors(e))

	// hand a slice of channels with no errors and observe no aggregate error
	var chans []<-chan error
	for i := 0; i < 10; i++ {
		e = make(chan error)
		close(e)
		chans = append(chans, e)
	}

	assert.NoError(aggregateErrors(chans...))

	// now try it if the channels have errors in them.
	// first a single error.
	e = make(chan error, 1)
	e <- errors.New("foo")
	close(e)
	assert.Error(aggregateErrors(e))

	chans = chans[:0] // clear the slice
	// a slice of channels with one error
	for i := 0; i < 10; i++ {
		e = make(chan error, 2)
		e <- errors.New("foo")
		close(e)
		chans = append(chans, e)
	}

	assert.Error(aggregateErrors(chans...))

	// finally run a test with a lot of errors in a few channel
	chans = chans[:0]
	for i := 0; i < 5; i++ {
		e = make(chan error, 12)
		for i := 0; i < 10; i++ {
			e <- errors.New("foo")
		}

		close(e)
		chans = append(chans, e)
	}

	assert.Error(aggregateErrors(chans...))
}
