package util

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mongodb/grip"
)

// ArchiveContentFile represents a tar file on disk.
type ArchiveContentFile struct {
	Path string
	Info os.FileInfo
	err  error
}

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

		for _, includePattern := range includes {
			dir, filematch := filepath.Split(includePattern)
			dir = filepath.Join(rootPath, dir)
			exists, err := FileExists(dir)
			if err != nil {
				outputChan <- ArchiveContentFile{err: err}
				continue
			}
			if !exists {
				continue
			}

			if ctx.Err() != nil {
				outputChan <- ArchiveContentFile{err: errors.New("archive creation operation canceled")}
				return
			}

			var walk filepath.WalkFunc

			if filematch == "**" {
				walk = func(path string, info os.FileInfo, err error) error {
					outputChan <- ArchiveContentFile{path, info, nil}
					return err
				}
				_ = filepath.Walk(dir, walk)
			} else if strings.Contains(filematch, "**") {
				globSuffix := filematch[2:]
				walk = func(path string, info os.FileInfo, err error) error {
					if strings.HasSuffix(filepath.Base(path), globSuffix) {
						outputChan <- ArchiveContentFile{path, info, nil}
					}
					return err
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
							outputChan <- ArchiveContentFile{path, info, nil}
						}
					}
					return err
				}
				_ = filepath.Walk(rootPath, walk)
			}
		}
	}()

	return outputChan
}
