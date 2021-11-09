package lru

import (
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
)

// FileObject is a simple struct that tracks data
type FileObject struct {
	Size  int
	Time  time.Time
	Path  string
	index int
}

// NewFile constructs a FileObject from its pathname and an
// os.FileInfo object. If the object is a directory, this method does
// not sum the total size of the directory.
func NewFile(fn string, info os.FileInfo) *FileObject {
	return &FileObject{
		Path: fn,
		Time: info.ModTime(),
		Size: int(info.Size()),
	}
}

// Update refreshs the objects data, and sums the total size of all
// objects in a directory if the object refers to a directory.
func (f *FileObject) Update() error {
	stat, err := os.Stat(f.Path)
	if os.IsNotExist(err) {
		return errors.Errorf("file %s no longer exists", f.Path)
	}

	f.Time = stat.ModTime()

	if stat.IsDir() {
		size, err := dirSize(f.Path)
		if err != nil {
			return errors.Wrapf(err, "problem finding size of directory %d", f.Path)
		}

		f.Size = int(size)
	} else {
		f.Size = int(stat.Size())
	}

	return nil
}

// Remove deletes the file object, recursively if necessary.
func (f *FileObject) Remove() error {
	return os.RemoveAll(f.Path)
}

func dirSize(path string) (int64, error) {
	var size int64

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			size += info.Size()
		}

		return nil
	})

	if err != nil {
		return 0, errors.Wrapf(err, "problem getting size of %s", path)
	}

	return size, nil
}
