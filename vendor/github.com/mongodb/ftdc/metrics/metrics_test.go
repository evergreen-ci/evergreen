package metrics

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/ftdc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func GetDirectoryOfFile() string {
	_, file, _, _ := runtime.Caller(1)

	return filepath.Dir(file)
}

func TestCollectRuntime(t *testing.T) {
	dir, err := ioutil.TempDir(filepath.Join(filepath.Dir(GetDirectoryOfFile()), "build"), "ftdc-")
	require.NoError(t, err)

	defer func() {
		if err = os.RemoveAll(dir); err != nil {
			fmt.Println(err)
		}
	}()

	t.Run("CollectData", func(t *testing.T) {
		opts := CollectOptions{
			OutputFilePrefix: filepath.Join(dir, fmt.Sprintf("sysinfo.%d.%s",
				os.Getpid(),
				time.Now().Format("2006-01-02.15-04-05"))),
			SampleCount:        10,
			FlushInterval:      time.Second,
			CollectionInterval: time.Millisecond,
			SkipProcess:        true,
			SkipSystem:         true,
		}
		var cancel context.CancelFunc
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = CollectRuntime(ctx, opts)
		require.NoError(t, err)
	})
	t.Run("ReadData", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var files []os.FileInfo
		files, err = ioutil.ReadDir(dir)
		require.NoError(t, err)
		assert.True(t, len(files) >= 1)

		total := 0
		for idx, info := range files {
			t.Run(fmt.Sprintf("FileNo.%d", idx), func(t *testing.T) {
				path := filepath.Join(dir, info.Name())
				var f *os.File
				f, err = os.Open(path)
				require.NoError(t, err)
				defer func() { assert.NoError(t, f.Close()) }()
				iter := ftdc.ReadMetrics(ctx, f)
				counter := 0
				for iter.Next() {
					counter++
					doc := iter.Document()
					assert.NotNil(t, doc)
					require.Equal(t, 3, doc.Len())
				}
				require.NoError(t, iter.Err())
				total += counter
			})
		}
	})
	t.Run("CollectAllData", func(t *testing.T) {
		if strings.Contains(os.Getenv("EVR_TASK_ID"), "race") {
			t.Skip("evergreen environment inconsistent")
		}
		// this test runs without the skips, which are
		// expected to be less reliable in different environment
		opts := CollectOptions{
			OutputFilePrefix: filepath.Join(dir, fmt.Sprintf("complete.%d.%s",
				os.Getpid(),
				time.Now().Format("2006-01-02.15-04-05"))),
			SampleCount:        100,
			FlushInterval:      time.Second,
			CollectionInterval: time.Millisecond,
		}
		var cancel context.CancelFunc
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err = CollectRuntime(ctx, opts)
		require.NoError(t, err)
	})
}
