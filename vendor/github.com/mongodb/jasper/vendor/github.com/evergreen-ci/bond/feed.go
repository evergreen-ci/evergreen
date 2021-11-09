package bond

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// ArtifactsFeed represents the entire structure of the MongoDB build information feed.
// See http://downloads.mongodb.org/full.json for an example.
type ArtifactsFeed struct {
	Versions []*ArtifactVersion

	mutex sync.RWMutex
	table map[string]*ArtifactVersion
	dir   string
	path  string
}

// GetArtifactsFeed parses a ArtifactsFeed object from a file on the file system.
// This operation will automatically refresh the feed from
// http://downloads.mongodb.org/full.json if the modification time of
// the file on the file system is more than 48 hours old.
func GetArtifactsFeed(ctx context.Context, path string) (*ArtifactsFeed, error) {
	feed, err := NewArtifactsFeed(path)
	if err != nil {
		return nil, errors.Wrap(err, "problem building feed")
	}

	if err := feed.Populate(ctx, 4*time.Hour); err != nil {
		return nil, errors.Wrap(err, "problem getting feed data")
	}

	return feed, nil
}

// NewArtifactsFeed takes the path of a file and returns an empty
// ArtifactsFeed object. You may specify an empty string as an argument
// to return a feed object homed on a temporary directory.
func NewArtifactsFeed(path string) (*ArtifactsFeed, error) {
	f := &ArtifactsFeed{
		table: make(map[string]*ArtifactVersion),
		path:  path,
	}

	if path == "" {
		// no value for feed, let's write it to the tempDir
		tmpDir, err := ioutil.TempDir("", "mongodb-downloads")
		if err != nil {
			return nil, err
		}

		f.dir = tmpDir
		f.path = filepath.Join(tmpDir, "full.json")
	} else if strings.HasSuffix(path, ".json") {
		f.dir = filepath.Dir(path)
		f.path = path
	} else {
		f.dir = path
		f.path = filepath.Join(f.dir, "full.json")
	}

	if stat, err := os.Stat(f.path); !os.IsNotExist(err) && stat.IsDir() {
		// if the thing we think should be the json file
		// exists but isn't a file (i.e. directory,) then this
		// should be an error.
		return nil, errors.Errorf("path %s not a json file  directory", path)
	}

	return f, nil
}

// Populate updates the local copy of the full feed in the the feed's
// cache if the local file doesn't exist or is older than the
// specified TTL. Additional Populate parses the data feed, using the
// Reload method.
func (feed *ArtifactsFeed) Populate(ctx context.Context, ttl time.Duration) error {
	data, err := CacheDownload(ctx, ttl, "http://downloads.mongodb.org/full.json", feed.path, false)

	if err != nil {
		return errors.Wrap(err, "problem getting feed data")
	}

	if err = feed.Reload(data); err != nil {
		return errors.Wrap(err, "problem reloading feed")
	}

	return nil
}

// Reload takes the content of the full.json file and loads this data
// into the current ArtifactsFeed object, overwriting any existing data.
func (feed *ArtifactsFeed) Reload(data []byte) error {
	// file exists, remove it if it's more than 48 hours old.
	feed.mutex.Lock()
	defer feed.mutex.Unlock()

	err := json.Unmarshal(data, feed)
	if err != nil {
		return errors.Wrap(err, "problem converting data from json")
	}

	// this is a reload rather than a new load, and we shoiuld
	if len(feed.table) > 0 {
		feed.table = make(map[string]*ArtifactVersion)
	}

	for _, version := range feed.Versions {
		feed.table[version.Version] = version
		version.refresh()
	}

	return err
}

// GetVersion takes a version string and returns the entire Artifacts version.
// The second value indicates if that release exists in the current feed.
func (feed *ArtifactsFeed) GetVersion(release string) (*ArtifactVersion, bool) {
	feed.mutex.RLock()
	defer feed.mutex.RUnlock()

	version, ok := feed.table[release]
	return version, ok
}

// GetLatestArchive given a release series (e.g. 3.2, 3.0, or 3.0),
// return the URL of the "latest" (e.g. nightly) build archive. These
// builds are atypical, and given how they're produced, may not
// necessarily reflect the most recent released or unreleased changes on a branch.
func (feed *ArtifactsFeed) GetLatestArchive(series string, options BuildOptions) (string, error) {
	series = coerceSeries(series)

	if options.Debug {
		return "", errors.New("debug symbols are not valid for nightly releases")
	}

	version, ok := feed.GetVersion(series + ".0")
	if !ok {
		return "", errors.Errorf("there is no .0 release for series '%s' in the feed", series)
	}

	dl, err := version.GetDownload(options)
	if err != nil {
		return "", errors.Wrapf(err, "problem fetching download information for series '%s'", series)
	}

	// if it's a dev version: then the branch name is in the file
	// name, and we just take the latest from master
	seriesNum, err := strconv.Atoi(string(series[2]))
	if err != nil {
		// this should be unreachable, because invalid
		// versions won't have yielded results from the feed
		// op
		return "", errors.Wrapf(err, "version specification is invalid")
	}

	if seriesNum%2 == 1 {
		return strings.Replace(dl.Archive.URL, version.Version, "latest", -1), nil
	}

	// if it's a stable version we just replace the version with the word latest.
	return strings.Replace(dl.Archive.URL, version.Version, "v"+series+"-latest", -1), nil
}

// GetCurrentArchive is a helper to download the latest stable release for a specific series.
func (feed *ArtifactsFeed) GetCurrentArchive(series string, options BuildOptions) (string, error) {
	feed.mutex.RLock()
	defer feed.mutex.RUnlock()

	version, err := feed.GetStableRelease(series)
	if err != nil {
		return "", errors.Wrap(err, "could not find version for: "+series)
	}

	dl, err := version.GetDownload(options)
	if err != nil {
		return "", errors.Wrap(err, "problem finding version")
	}

	return dl.Archive.URL, nil

}

// GetStableRelease returns the latest official release for a specific series.
func (feed *ArtifactsFeed) GetStableRelease(series string) (*ArtifactVersion, error) {
	series = coerceSeries(series)

	if series == "2.4" {
		version, ok := feed.GetVersion("2.4.14")
		if !ok {
			return nil, errors.Errorf("could not find current version 2.4.14")
		}
		return version, nil
	}

	for _, version := range feed.Versions {
		if version.Current && strings.HasPrefix(version.Version, series) {
			return version, nil
		}
	}

	return nil, errors.Errorf("could not find a current version for series: %s", series)
}

// GetArchives provides an iterator for all archives given a list of
// releases (versions) for a specific set of build operations.
// Returns channels of urls (strings) and errors. Read from the error channel,
// after completing all results.
func (feed *ArtifactsFeed) GetArchives(releases []string, options BuildOptions) (<-chan string, <-chan error) {
	output := make(chan string)
	errOut := make(chan error)

	go func() {
		catcher := grip.NewCatcher()
		for _, rel := range releases {
			// this is a series, have to handle it differently
			hasLatest := strings.Contains(rel, "latest")
			if len(rel) == 3 || hasLatest {
				if hasLatest {
					rel = strings.Split(rel, "-")[0]
				}

				url, err := feed.GetLatestArchive(rel, options)
				if err != nil {
					catcher.Add(err)
					continue
				}
				output <- url
				continue
			}

			if strings.HasSuffix(rel, "-current") || strings.HasSuffix(rel, "-stable") {
				rel = strings.Split(rel, "-")[0]
				url, err := feed.GetCurrentArchive(rel, options)
				if err != nil {
					catcher.Add(err)
					continue
				}
				output <- url
				continue
			}

			version, ok := feed.GetVersion(rel)
			if !ok {
				catcher.Add(errors.Errorf("no version defined for %s", rel))
				continue
			}
			dl, err := version.GetDownload(options)
			if err != nil {
				catcher.Add(err)
				continue
			}

			if options.Debug {
				output <- dl.Archive.Debug
				continue
			}
			output <- dl.Archive.URL
		}
		close(output)
		if catcher.HasErrors() {
			errOut <- catcher.Resolve()
		}
		close(errOut)
	}()

	return output, errOut
}

func coerceSeries(series string) string {
	if series[0] == 'v' {
		series = series[1:]
	}

	if len(series) > 3 {
		series = series[:3]
	}

	return series
}
