package lru

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mongodb/grip"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CacheSuite struct {
	tempDir string
	cache   *Cache
	require *require.Assertions
	suite.Suite
}

func TestCacheSuite(t *testing.T) {
	suite.Run(t, new(CacheSuite))
}

func (s *CacheSuite) SetupSuite() {
	s.require = s.Require()
	dir, err := ioutil.TempDir("", uuid.NewV4().String())
	s.require.NoError(err)
	s.tempDir = dir
}

func (s *CacheSuite) TearDownSuite() {
	grip.CatchError(os.RemoveAll(s.tempDir))
}

func (s *CacheSuite) SetupTest() {
	s.cache = NewCache()
	s.require.Len(s.cache.table, 0)
}

func (s *CacheSuite) TestInitialStateOfCacheObjectIsEmpty() {
	s.Equal(0, s.cache.size)
	s.Len(s.cache.table, 0)
	s.Equal(0, s.cache.heap.Len())

	s.Equal(0, s.cache.Count())
	s.Equal(0, s.cache.Size())
}

func (s *CacheSuite) TestAddFileThatDoeNotExistResultsInError() {
	s.Error(s.cache.AddFile(filepath.Join(s.tempDir, "DOES-NOT-EXIST")))
}

func (s *CacheSuite) TestAddDirectoryThatExistsSucceeds() {
	s.NoError(s.cache.AddFile(s.tempDir))
}

func (s *CacheSuite) TestAddRejectsFilesThatAlreadyExist() {
	s.NoError(s.cache.AddFile(s.tempDir))

	for i := 0; i < 40; i++ {
		s.Error(s.cache.AddFile(s.tempDir))
	}
}

func (s *CacheSuite) TestMutlithreadedFileAdds() {
	s.Equal(0, s.cache.Count())
	s.Equal(0, s.cache.Size())

	wg := &sync.WaitGroup{}
	for i := 0; i < 40; i++ {
		fn := filepath.Join(s.tempDir, uuid.NewV4().String())
		s.NoError(ioutil.WriteFile(fn, []byte(fmt.Sprintf("in %s is it %d", fn, i)), 0644))
		wg.Add(1)
		go func(f string) {
			s.NoError(s.cache.AddFile(f))
			wg.Done()
		}(fn)
	}
	wg.Wait()

	s.Equal(40, s.cache.Count())
	s.Equal(5710, s.cache.Size())
}

func (s *CacheSuite) TestAddStatRejectsFilesWithNilStats() {
	s.Error(s.cache.AddStat("foo", nil))
}

func (s *CacheSuite) TestUpdateRejectsRecordsThatAreNotLocal() {
	fn := filepath.Join(s.tempDir, "foo")
	fobj := &FileObject{
		Path: fn,
	}

	s.NoError(ioutil.WriteFile(fn, []byte("foo"), 0644))

	s.Error(s.cache.Update(fobj))
	s.NoError(s.cache.Add(fobj))
	s.NoError(s.cache.Update(fobj))
}
