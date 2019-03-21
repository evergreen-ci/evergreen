package bond

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// BuildCatalog is a structure that represents a group of MongoDB
// artifacts managed by bond, and provides an interface for retrieving
// artifacts.
type BuildCatalog struct {
	Path  string
	table map[BuildInfo]string
	feed  *ArtifactsFeed
	mutex sync.RWMutex
}

// NewCatalog populates and returns a BuildCatalog object from a given path.
func NewCatalog(ctx context.Context, path string) (*BuildCatalog, error) {
	var err error
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, errors.Wrap(err, "problem resolving absolute path")
	}

	contents, err := getContents(path)
	if err != nil {
		return nil, errors.Wrap(err, "could not find contents")
	}

	feed, err := GetArtifactsFeed(ctx, path)
	if err != nil {
		return nil, errors.Wrap(err, "could not find build feed")
	}

	cache := &BuildCatalog{
		Path:  path,
		feed:  feed,
		table: map[BuildInfo]string{},
	}

	catcher := grip.NewCatcher()
	for _, obj := range contents {
		if !obj.IsDir() {
			continue
		}

		if !strings.HasPrefix(obj.Name(), "mongodb-") {
			continue
		}

		fileName := filepath.Join(path, obj.Name())

		if err := cache.Add(fileName); err != nil {
			catcher.Add(err)
			continue
		}
	}

	if catcher.HasErrors() {
		return nil, errors.Wrapf(catcher.Resolve(),
			"problem building build catalog from path: %s", path)
	}

	return cache, nil
}

// Add adds a build to the catalog, and returns an error if it's not a
// valid build. The build file name must be a part of the path
// specified when creating the BuildCatalog object, otherwise Add will
// not add this item to the cache and return an error and .
func (c *BuildCatalog) Add(fileName string) error {
	fileName, err := filepath.Abs(fileName)
	if err != nil {
		return errors.Wrapf(err, "problem finding absolute path of %s object", fileName)
	}

	if !strings.HasPrefix(fileName, c.Path) {
		return errors.Errorf("cannot add %s to '%s' cache because it is not in the same root",
			fileName, c.Path)
	}

	info, err := GetInfoFromFileName(fileName)
	if err != nil {
		return errors.Wrap(err, "problem collecting information about build")
	}

	err = validateBuildArtifacts(fileName, info.Version)
	if err != nil {
		return errors.Wrapf(err, "problem validating contents of %s", fileName)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	if _, ok := c.table[info]; ok {
		return errors.Errorf("path %s exists in cache", fileName)
	}

	c.table[info] = fileName

	return nil
}

// Contents returns a copy of the contents of the catalog.
func (c *BuildCatalog) Contents() map[BuildInfo]string {
	output := map[BuildInfo]string{}
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	for k, v := range c.table {
		output[k] = v
	}

	return output
}

func (c *BuildCatalog) String() string {
	inverted := map[string]BuildInfo{}

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	for k, v := range c.table {
		inverted[v] = k
	}
	if len(inverted) != len(c.table) {
		return "{ 'error': 'invalid catalog data' }"
	}

	out, err := json.MarshalIndent(inverted, "", "   ")
	if err != nil {
		return fmt.Sprintf("%+v", inverted)
	}

	return string(out)
}

// Get returns the path to a build in the BuildCatalog based on the
// parameters presented. Returns an error if a build matching the
// parameters specified does not exist in the cache.
func (c *BuildCatalog) Get(version, edition, target, arch string, debug bool) (string, error) {
	if strings.Contains(version, "current") {
		v, err := c.feed.GetStableRelease(version)
		if err != nil {
			return "", errors.Wrapf(err,
				"could not determine current stable release for %s series", version)
		}

		version = v.Version
	} else if strings.Contains(version, "latest") {
		version = fmt.Sprintf("%s-latest", coerceSeries(version))
	}

	if strings.Contains(target, "auto") {
		t, err := getDistro()
		if err != nil {

			if runtime.GOOS == "darwin" {
				target = "osx"
			} else {
				target = runtime.GOOS
			}

			grip.Warning(message.WrapError(err, message.NewFormatted("could not determine distro, falling back to %s", target)))
		} else {
			target = t
		}
	}

	info := BuildInfo{
		Version: version,
		Options: BuildOptions{
			Target:  target,
			Arch:    MongoDBArch(arch),
			Edition: MongoDBEdition(edition),
			Debug:   debug,
		},
	}

	// TODO consider if we want to validate against bad or invalid
	// options; potentially by extending the buildinfo validation method.

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	path, ok := c.table[info]
	if !ok {
		return "", errors.Errorf("could not find version %s, edition %s, target %s, arch %s in %s",
			version, edition, target, arch, c.Path)
	}

	return path, nil
}

func getContents(path string) ([]os.FileInfo, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []os.FileInfo{}, errors.Errorf("path %s does not exist", path)
	}

	contents, err := ioutil.ReadDir(path)
	if err != nil {
		return []os.FileInfo{}, errors.Wrapf(err, "problem fetching contents of %s", path)
	}

	if len(contents) == 0 {
		return []os.FileInfo{}, errors.Errorf("path %s is empty", path)
	}

	return contents, nil
}

func validateBuildArtifacts(path, version string) error {
	path = filepath.Join(path, "bin")

	contents, err := getContents(path)
	if err != nil {
		return errors.Wrapf(err, "problem finding contents for %s", version)
	}

	pkg := make(map[string]struct{})
	for _, info := range contents {
		pkg[info.Name()] = struct{}{}
	}

	catcher := grip.NewCatcher()
	for _, bin := range []string{"mongod", "mongos"} {
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}

		if _, ok := pkg[bin]; !ok {
			catcher.Add(errors.Errorf("binary %s is missing from %s for %s",
				bin, path, version))
		}
	}

	return catcher.Resolve()
}
