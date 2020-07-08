package lru

import (
	"strings"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Prune removes files (when dryRun is false), from the file system
// until the total size of the cache is less than the maxSize (in
// bytes.)
func (c *Cache) Prune(maxSize int, exclude []string, dryRun bool) error {
	catcher := grip.NewCatcher()

	for {
		if c.underQuota(maxSize) {
			break
		}

		if err := c.prunePass(exclude, dryRun); err != nil {
			grip.Noticef("cache pruning ended early due to error, (size=%d, count=%d)",
				c.Size(), c.Count())
			catcher.Add(err)
		}
	}

	return catcher.Resolve()
}

func (c *Cache) underQuota(maxSize int) bool {
	if c.Count() == 0 {
		return true
	}

	size := c.Size()
	if size <= maxSize {
		return true
	}

	return false
}

func (c *Cache) prunePass(exclude []string, dryRun bool) error {
	f, err := c.Pop()
	if err != nil {
		return errors.Wrap(err, "problem retrieving item from cache")
	}

	for _, ex := range exclude {
		if strings.HasSuffix(f.Path, ex) {
			grip.Infof("file '%s' is excluded from pruning", f.Path)
			return nil
		}
	}

	if dryRun {
		grip.Noticef("[dry-run]: would delete '%s' (%dMB)", f.Path,
			f.Size/1024/1024)
		return nil
	}

	if err := f.Remove(); err != nil {
		return errors.Wrap(err, "problem removing item")
	}

	grip.Infof("removed '%s' (%dMB) from the cache", f.Path, f.Size/1024/1024)
	return nil
}
