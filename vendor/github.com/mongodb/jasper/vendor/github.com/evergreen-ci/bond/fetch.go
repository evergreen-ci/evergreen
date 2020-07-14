package bond

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func createDirectory(path string) error {
	stat, err := os.Stat(path)

	if os.IsNotExist(err) {
		err := os.MkdirAll(path, 0755)
		if err != nil {
			return errors.Wrapf(err, "problem crating directory %s", path)
		}
		grip.Noticeln("created directory:", path)
	} else if !stat.IsDir() {
		return errors.Errorf("%s exists and is not a directory", path)
	}

	return nil
}

// CacheDownload downloads a resource (url) into a file (path); if the
// file already exists CaceheDownlod does not download a new copy of
// the file, unless local file is older than the ttl, or the force
// option is specified. CacheDownload returns the contents of the file.
func CacheDownload(ctx context.Context, ttl time.Duration, url, path string, force bool) ([]byte, error) {
	if ttl == 0 {
		force = true
	}

	if stat, err := os.Stat(path); !os.IsNotExist(err) {
		age := time.Since(stat.ModTime())

		if (ttl > 0 && age > ttl) || force {
			grip.Infof("removing stale (%s) file (%s)", age, path)
			if err = os.Remove(path); err != nil {
				return nil, errors.Wrap(err, "problem removing stale feed.")
			}
		}
	}

	// TODO: we're effectively reading the file into memory twice
	// to write it to disk and read it out again.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := DownloadFile(ctx, url, path)
		if err != nil {
			return nil, err
		}
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// DownloadFile downloads a resource (url) into a file specified by
// fileName. Also creates enclosing directories as needed.
func DownloadFile(ctx context.Context, url, fileName string) error {
	if err := createDirectory(filepath.Dir(fileName)); err != nil {
		return errors.Wrapf(err, "problem creating enclosing directory for %s", fileName)
	}

	if _, err := os.Stat(fileName); !os.IsNotExist(err) {
		return errors.Errorf("'%s' file exists", fileName)
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(err, "problem building request")
	}
	req = req.WithContext(ctx)

	output, err := os.Create(fileName)
	if err != nil {
		return errors.Wrapf(err, "could not create file for package '%s'", fileName)
	}
	defer output.Close()

	client := GetHTTPClient()
	defer PutHTTPClient(client)

	grip.Noticeln("downloading:", fileName)
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "problem downloading file")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		grip.Warning(os.Remove(fileName))
		return errors.Errorf("encountered error %d (%s) for %s", resp.StatusCode, resp.Status, url)
	}

	n, err := io.Copy(output, resp.Body)
	if err != nil {
		grip.Warning(os.Remove(fileName))
		return errors.Wrapf(err, "problem writing %s to file %s", url, fileName)
	}

	grip.Debugf("%d bytes downloaded. (%s)", n, fileName)
	return nil
}
