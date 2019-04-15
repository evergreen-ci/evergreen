package recall

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type DownloadJobSuite struct {
	job     *DownloadFileJob
	require *require.Assertions
	tempDir string
	suite.Suite
}

func TestDownloadJobSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(DownloadJobSuite))
}

func (s *DownloadJobSuite) SetupSuite() {
	var err error
	s.require = s.Require()
	// s.tempDir, err = ioutil.TempDir("", uuid.NewV4().String())
	s.tempDir, err = ioutil.TempDir("", "")
	s.require.NoError(err)
}

func (s *DownloadJobSuite) TearDownSuite() {
	grip.Warningln("leaking tempdir for quicker tests:", s.tempDir)
	// err := os.RemoveAll(s.tempDir)
	// s.require.NoError(err)
}

func (s *DownloadJobSuite) SetupTest() {
	s.job = newDownloadJob()
}

func (s *DownloadJobSuite) TestUrlSetterAndValidatorErrorsWithInvaludUrls() {
	values := []string{
		"htp://foo.example.com",
		"ftp://foo.example.com",
		"foo.example.com",
		"foo.example",
		"example.com",
		"com.example.foo://http",
	}
	for _, v := range values {
		s.Error(s.job.setURL(v))
		s.Equal("", s.job.URL)
		s.Equal("", s.job.FileName)
		j, err := NewDownloadJob(v, "foo", false)
		s.Nil(j)
		s.Error(err)
	}

}

func (s *DownloadJobSuite) TestUrlSetterAndValidorErrorsWithoutFileNameComponent() {
	url := "http://foo.example.net/"

	values := []string{
		"",
		"/",
		"/foo/bar/",
		"/foo/bar/baz/",
		"foo/bar/",
		"foo/bar/baz/",
	}

	for _, v := range values {
		s.Error(s.job.setURL(url + v))
		s.Equal("", s.job.URL)
		s.Equal("", s.job.FileName)
		j, err := NewDownloadJob(url+v, "foo", false)
		s.Nil(j)
		s.Error(err)
	}
}

func (s *DownloadJobSuite) TestUrlSetterWithValidFileName() {
	url := "http://foo.example.net/"

	values := []string{
		"/foo.tgz",
		"/foo.zip",
		"/foo",
		"/bar/foo.tgz",
		"/bar/foo.zip",
		"/bar/foo",
		"foo.tgz",
		"foo.zip",
		"foo",
		"bar/foo.tgz",
		"bar/foo.zip",
		"bar/foo",
	}

	for _, v := range values {
		path := url + v
		s.NoError(s.job.setURL(path))
		s.NotEqual("", s.job.URL)
		s.Equal(filepath.Base(v), s.job.FileName)
	}
}

func (s *DownloadJobSuite) TestTarGzExtensionSpecialCase() {
	url := "http://foo.example.net/"

	values := []string{
		"/foo.tar.gz",
		"/bar/foo.tar.gz",
		"foo.tar.gz",
		"bar/foo.tar.gz",
	}

	for _, v := range values {
		path := url + v
		s.NoError(s.job.setURL(path))
		s.NoError(s.job.setURL(path))
		s.NotEqual("", s.job.URL)
		s.True(strings.HasSuffix(s.job.FileName, ".tgz"))
	}
}

func (s *DownloadJobSuite) TestSetDirectoryToFileReturnsError() {
	path := "makefile"
	s.Error(s.job.setDirectory(path))
	s.Equal("", s.job.Directory)

	j, err := NewDownloadJob("http://example.net/foo.tgz", path, false)
	s.Error(err)
	s.Nil(j)
}

func (s *DownloadJobSuite) TestSetDirectorySucceedsIfPathDoesNotExist() {
	name := "../makefile-DOES-NOT-EXIST"
	s.NoError(s.job.setDirectory(name))

	s.Equal(name, s.job.Directory)
}

func (s *DownloadJobSuite) TestSetDirectorySucceedsIfPathExistsAndIsDirectory() {
	name := "../build"
	s.NoError(s.job.setDirectory(name))

	s.Equal(name, s.job.Directory)
}

func (s *DownloadJobSuite) TestConstructorSetsDependencyBasedOnForceParameter() {
	url := "http://example.net/foo.tgz"
	path := "../build"

	j, err := NewDownloadJob(url, path, true)
	s.NoError(err)
	s.Equal(dependency.NewAlways(), j.Dependency())

	j, err = NewDownloadJob(url, path, false)
	s.NoError(err)
	s.Equal(dependency.NewCreatesFile("../build/foo.tgz").Type(), j.Dependency().Type())
}

func (s *DownloadJobSuite) TestErrorHandler() {
	s.False(s.job.HasErrors())
	s.job.handleError(nil)
	s.False(s.job.HasErrors())

	s.job.handleError(errors.New("foo"))
	s.True(s.job.HasErrors())

}

func (s *DownloadJobSuite) TestJobSmokeTests() {
	for _, url := range []string{
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-3.2.11.tgz",
		"https://fastdl.mongodb.org/win32/mongodb-win32-x86_64-3.2.11.zip",
	} {
		fn := filepath.Base(url)
		j, err := NewDownloadJob(url, s.tempDir, false)
		s.NoError(err)

		j.Run(context.TODO())
		s.NoError(j.Error())

		_, err = os.Stat(filepath.Join(s.tempDir, fn))
		s.False(os.IsNotExist(err))
		stat, err := os.Stat(filepath.Join(s.tempDir, fn[:len(fn)-4]))
		s.False(os.IsNotExist(err))
		s.True(stat.IsDir())
	}
}

func (s *DownloadJobSuite) TestJobWithFileThatDoesNotExistReportsError() {
	for _, url := range []string{
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-3.2.11.zip",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-2.8.11.tgz",
	} {
		j, err := NewDownloadJob(url, s.tempDir, true)
		s.NoError(err)

		j.Run(context.TODO())
		s.Error(j.Error())
	}
}

func (s *DownloadJobSuite) TestInvalidExtensionsReturnErrors() {
	for _, url := range []string{
		"https://downloads.mongodb.org/default.json",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-3.2.11.tgz.sig",
	} {
		j, err := NewDownloadJob(url, s.tempDir, true)
		s.NoError(err)

		j.Run(context.TODO())
		s.Error(j.Error())
	}
}

func (s *DownloadJobSuite) TestNoopCaseIfDependencyIsSatisfiedAndForceIsNotSet() {
	url := "https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-3.2.10.tgz"
	fn := filepath.Base(url)

	j, err := NewDownloadJob(url, s.tempDir, false)
	s.NoError(err)

	j.SetDependency(dependency.NewCreatesFile("/etc"))
	s.Equal(j.Dependency().State(), dependency.Passed)
	j.Run(context.TODO())
	s.NoError(j.Error())

	_, err = os.Stat(fn)
	s.True(os.IsNotExist(err))
}

func (s *DownloadJobSuite) TestIfDependencyIsSatisfiedAndForceIsSetThereIsNoNoop() {
	url := "https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-3.2.9.tgz"
	fn := filepath.Base(url)

	j, err := NewDownloadJob(url, s.tempDir, true)
	s.NoError(err)

	j.SetDependency(dependency.NewCreatesFile(s.tempDir))
	s.Equal(j.Dependency().State(), dependency.Passed)
	j.Run(context.TODO())
	s.NoError(j.Error())

	_, err = os.Stat(fn)
	s.True(os.IsNotExist(err))
}

//
// Standalone Test Cases:
//

func TestJobRegistry(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	var names []string
	for n := range registry.JobTypeNames() {
		names = append(names, n)
	}

	assert.Len(names, 1)

	jobType := "bond-recall-download-file"
	j, err := registry.GetJobFactory(jobType)
	job := j()
	assert.NoError(err)
	assert.Implements((*amboy.Job)(nil), job)
	assert.Equal(job.Type().Name, jobType)
}
