// Package lru provides a tool to prune files from a cache based on
// LRU.
//
// lru implements a cache structure that tracks the size and
// (modified*) time of a file.
//
// * future versions of lru may use different time method to better
// approximate usage time rather than modification time.
package lru

import (
	"container/heap"
	"os"
	"sync"

	"github.com/pkg/errors"
)

// Cache provides tools to maintain an cache of file system objects,
// maintained on a least-recently-used basis. Internally, files are
// stored internally as a heap.
type Cache struct {
	size  int
	heap  fileObjectHeap
	mutex sync.RWMutex
	table map[string]*FileObject
}

// NewCache returns an initalized but unpopulated cache. Use the
// DirectoryContents and TreeContents constructors to populate a
// cache.
func NewCache() *Cache {
	return &Cache{
		table: make(map[string]*FileObject),
	}
}

// Size returns the total size of objects in the cache.
func (c *Cache) Size() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.size
}

// Count returns the total number of objects in the cache.
func (c *Cache) Count() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return len(c.heap)
}

// AddStat takes the full (absolute) path to a file and an os.FileInfo
// object and and constructs the FileObject and adds it to the
// cache. AddStat returns an error if the stat is invalid, or the file
// already exists in the chace.
func (c *Cache) AddStat(fn string, stat os.FileInfo) error {
	if stat == nil {
		return errors.Errorf("file %s does not have a valid stat", fn)
	}

	f := &FileObject{
		Path: fn,
		Size: int(stat.Size()),
		Time: stat.ModTime(),
	}

	if stat.IsDir() {
		size, err := dirSize(fn)
		if err != nil {
			return errors.Wrapf(err, "problem finding size of directory %d", fn)
		}

		f.Size = int(size)
	}

	return errors.Wrapf(c.Add(f), "problem adding file (%s) by info", fn)
}

// AddFile takes a fully qualified filename and adds it to the cache,
// returning an error if the file does not exist. AddStat returns an
// error if the stat is invalid, or the file already exists in the
// cache.
func (c *Cache) AddFile(fn string) error {
	stat, err := os.Stat(fn)
	if os.IsNotExist(err) {
		return errors.Wrapf(err, "file %s does not exist", fn)
	}

	return errors.Wrap(c.AddStat(fn, stat), "problem adding file")
}

// Add takes a defined FileObject and adds it to the cache, returning
// an error if the object already exists.
func (c *Cache) Add(f *FileObject) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, ok := c.table[f.Path]; ok {
		return errors.Errorf("cannot add object '%s' to cache: it already exists: %s",
			f.Path, "use Update() instead")
	}

	c.size += f.Size
	c.table[f.Path] = f

	heap.Push(&c.heap, f)

	return nil
}

// Update updates an existing item in the cache, returning an error if
// it is not in the cache.
func (c *Cache) Update(f *FileObject) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	existing, ok := c.table[f.Path]
	if !ok {
		return errors.Errorf("cannot update '%s' in cache: it does not exist: %s",
			f.Path, "use Add() instead")
	}

	c.size -= existing.Size
	c.size += f.Size

	f.index = existing.index
	c.table[f.Path] = f
	heap.Fix(&c.heap, f.index)

	return nil
}

// Pop removes and returns the oldest object in the cache.
func (c *Cache) Pop() (*FileObject, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.heap.Len() == 0 {
		return nil, errors.New("cache listing is empty")
	}

	f := heap.Pop(&c.heap).(*FileObject)
	c.size -= f.Size
	delete(c.table, f.Path)

	return f, nil
}

// Get returns an item from the cache by name. This does not impact
// the item's position in the cache.
func (c *Cache) Get(path string) (*FileObject, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	f, ok := c.table[path]
	if !ok {
		return nil, errors.Errorf("file '%s' does not exist in cache", path)
	}

	return f, nil
}

// Contents returns an iterator for
func (c *Cache) Contents() <-chan string {
	out := make(chan string)
	go func() {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		for fn := range c.table {
			out <- fn
		}

		close(out)
	}()
	return out
}
