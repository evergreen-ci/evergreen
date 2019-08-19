package jasper

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

var httpClientPool *sync.Pool

func init() {
	httpClientPool = &sync.Pool{
		New: func() interface{} {
			return &http.Client{}
		},
	}
}

// GetHTTPClient gets an HTTP client from the client pool.
func GetHTTPClient() *http.Client {
	return httpClientPool.Get().(*http.Client)
}

// PutHTTPClient returns the given HTTP client back to the pool.
func PutHTTPClient(client *http.Client) {
	httpClientPool.Put(client)
}

func sliceContains(group []string, name string) bool {
	for _, g := range group {
		if name == g {
			return true
		}
	}

	return false
}

func makeEnclosingDirectories(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(path, os.ModeDir|os.ModePerm); err != nil {
			return err
		}
	} else if !info.IsDir() {
		return errors.Errorf("'%s' already exists and is not a directory", path)
	}
	return nil
}

func writeFile(reader io.Reader, path string) error {
	if err := makeEnclosingDirectories(filepath.Dir(path)); err != nil {
		return errors.Wrap(err, "problem making enclosing directories")
	}

	file, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "problem creating file")
	}

	catcher := grip.NewBasicCatcher()
	if _, err := io.Copy(file, reader); err != nil {
		catcher.Add(errors.Wrap(err, "problem writing file"))
	}

	catcher.Add(errors.Wrap(file.Close(), "problem closing file"))

	return catcher.Resolve()
}
