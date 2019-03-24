package bond

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/grip"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type DownloaderSuite struct {
	dir     string
	require *require.Assertions
	suite.Suite
}

func TestDownloaderSuite(t *testing.T) {
	suite.Run(t, new(DownloaderSuite))
}

func (s *DownloaderSuite) SetupSuite() {
	var err error
	s.require = s.Require()
	s.dir, err = ioutil.TempDir("", uuid.NewV4().String())
	s.require.NoError(err)
	_, err = os.Stat(s.dir)
	s.require.NoError(os.MkdirAll(s.dir, 0700))
	s.require.True(!os.IsNotExist(err))
}

func (s *DownloaderSuite) TearDownSuite() {
	grip.CatchCritical(os.RemoveAll(s.dir))
}

func (s *DownloaderSuite) TestCreateDirectory() {
	// the s.dir location exists when we start
	s.NoError(createDirectory(s.dir))
}

func (s *DownloaderSuite) TestCreateDirectoryThatDoesNotExist() {
	name := filepath.Join(s.dir, uuid.NewV4().String())
	_, err := os.Stat(name)
	s.True(os.IsNotExist(err))

	s.NoError(createDirectory(name))
	_, err = os.Stat(name)
	s.False(os.IsNotExist(err))
}

func (s *DownloaderSuite) TestCreateDirectoryErrorsIfPathIsAFile() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	name := filepath.Join(s.dir, uuid.NewV4().String())

	_, err := os.Stat(name)
	s.True(os.IsNotExist(err))

	s.NoError(ioutil.WriteFile(name, []byte("hi"), 0644))

	_, err = os.Stat(name)
	s.False(os.IsNotExist(err))

	s.Error(createDirectory(name))

	_, err = os.Stat(name)
	s.False(os.IsNotExist(err))

	// also check here that this error propagates to DownloadFile
	s.Error(DownloadFile(ctx, "foo", filepath.Join(name, "foo")))
	s.Error(DownloadFile(ctx, "foo", name))
}

func (s *DownloaderSuite) TestDownloadFileThatExists() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := DownloadFile(ctx, "http://example.net", filepath.Join(s.dir, uuid.NewV4().String()))
	s.NoError(err, fmt.Sprintf("%+v", err))
}

func (s *DownloaderSuite) TestDownloadFileThatDoesNotExist() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := DownloadFile(ctx, "http://example.net/DOES_NOT_EXIST", filepath.Join(s.dir, uuid.NewV4().String()))
	s.Error(err, fmt.Sprintf("%+v", err))
}
