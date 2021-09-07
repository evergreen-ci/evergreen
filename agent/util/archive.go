package util

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// ArchiveContentFile represents a tar file on disk.
type ArchiveContentFile struct {
	Path string
	Info os.FileInfo
	err  error
}

// FindContentsToArchive finds all files starting from the rootPath with the
// given inclusion and exclusion patterns.
func FindContentsToArchive(ctx context.Context, rootPath string, includes, excludes []string) ([]ArchiveContentFile, error) {
	out := []ArchiveContentFile{}
	catcher := grip.NewBasicCatcher()
	archiveContents, err := streamArchiveContents(ctx, rootPath, includes, excludes)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting archive contents")
	}
	for _, fn := range archiveContents {
		if fn.err != nil {
			catcher.Add(fn.err)
			continue
		}

		out = append(out, fn)
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return out, nil
}

func streamArchiveContents(ctx context.Context, rootPath string, includes, excludes []string) ([]ArchiveContentFile, error) {
	archiveContents := []ArchiveContentFile{}
	catcher := grip.NewCatcher()

	for _, includePattern := range includes {
		dir, filematch := filepath.Split(includePattern)
		dir = filepath.Join(rootPath, dir)

		if !utility.FileExists(dir) {
			continue
		}

		if ctx.Err() != nil {
			return nil, errors.New("archive creation operation canceled")
		}

		var walk filepath.WalkFunc

		if filematch == "**" {
			walk = func(path string, info os.FileInfo, err error) error {
				for _, ignore := range excludes {
					if match, _ := filepath.Match(ignore, path); match {
						return nil
					}
				}

				archiveContents = append(archiveContents, ArchiveContentFile{path, info, nil})
				return nil
			}
			catcher.Add(filepath.Walk(dir, walk))
		} else if strings.Contains(filematch, "**") {
			globSuffix := filematch[2:]
			walk = func(path string, info os.FileInfo, err error) error {
				if strings.HasSuffix(filepath.Base(path), globSuffix) {
					for _, ignore := range excludes {
						if match, _ := filepath.Match(ignore, path); match {
							return nil
						}
					}

					archiveContents = append(archiveContents, ArchiveContentFile{path, info, nil})
				}
				return nil
			}
			catcher.Add(filepath.Walk(dir, walk))
		} else {
			walk = func(path string, info os.FileInfo, err error) error {
				a, b := filepath.Split(path)
				if filepath.Clean(a) == filepath.Clean(dir) {
					match, err := filepath.Match(filematch, b)
					if err != nil {
						archiveContents = append(archiveContents, ArchiveContentFile{err: err})
					}
					if match {
						for _, ignore := range excludes {
							if exmatch, _ := filepath.Match(ignore, path); exmatch {
								return nil
							}
						}

						archiveContents = append(archiveContents, ArchiveContentFile{path, info, nil})
					}
				}
				return nil
			}
			catcher.Add(filepath.Walk(rootPath, walk))
		}
	}
	return archiveContents, catcher.Resolve()
}
