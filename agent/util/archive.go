package util

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
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
	for fn := range streamArchiveContents(ctx, rootPath, includes, excludes) {
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

func streamArchiveContents(ctx context.Context, rootPath string, includes, excludes []string) <-chan ArchiveContentFile {
	outputChan := make(chan ArchiveContentFile)

	go func() {
		defer close(outputChan)
		defer func() {
			panicErr := recovery.HandlePanicWithError(recover(), nil,
				"streaming archive contents")
			if panicErr != nil {
				outputChan <- ArchiveContentFile{err: panicErr}
			}
		}()
		for _, includePattern := range includes {
			dir, filematch := filepath.Split(includePattern)
			dir = filepath.Join(rootPath, dir)

			if !utility.FileExists(dir) {
				continue
			}

			if ctx.Err() != nil {
				outputChan <- ArchiveContentFile{err: errors.New("archive creation operation canceled")}
				return
			}

			var walk filepath.WalkFunc

			if filematch == "**" {
				walk = func(path string, info os.FileInfo, err error) error {
					for _, ignore := range excludes {
						if match, _ := filepath.Match(ignore, path); match {
							return nil
						}
					}

					outputChan <- ArchiveContentFile{path, info, nil}
					return nil
				}
				_ = filepath.Walk(dir, walk)
			} else if strings.Contains(filematch, "**") {
				globSuffix := filematch[2:]
				walk = func(path string, info os.FileInfo, err error) error {
					if strings.HasSuffix(filepath.Base(path), globSuffix) {
						for _, ignore := range excludes {
							if match, _ := filepath.Match(ignore, path); match {
								return nil
							}
						}

						outputChan <- ArchiveContentFile{path, info, nil}
					}
					return nil
				}
				_ = filepath.Walk(dir, walk)
			} else {
				walk = func(path string, info os.FileInfo, err error) error {
					a, b := filepath.Split(path)
					if filepath.Clean(a) == filepath.Clean(dir) {
						match, err := filepath.Match(filematch, b)
						if err != nil {
							outputChan <- ArchiveContentFile{err: err}
						}
						if match {
							for _, ignore := range excludes {
								if exmatch, _ := filepath.Match(ignore, path); exmatch {
									return nil
								}
							}

							outputChan <- ArchiveContentFile{path, info, nil}
						}
					}
					return nil
				}
				_ = filepath.Walk(rootPath, walk)
			}
		}
	}()

	return outputChan
}
