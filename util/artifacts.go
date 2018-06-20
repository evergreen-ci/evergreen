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
}

func FindContentsToArchive(ctx context.Context, rootPath string, includes, excludes []string) ([]ArchiveContentFile, error) {
	out := []ArchiveContentFile{}

	files, errs := streamArchiveContents(ctx, rootPath, includes, excludes)
	for fn := range files {
		out = append(out, fn)
	}

	err, ok := <-errs
	if ok && err != nil {
		return nil, err
	}

	return out, nil
}

func streamArchiveContents(ctx context.Context, rootPath string, includes, excludes []string) (<-chan ArchiveContentFile, <-chan error) {
	errChan := make(chan error)
	outputChan := make(chan ArchiveContentFile)

	go func() {
		defer close(errChan)
		defer close(outputChan)

		for _, includePattern := range includes {
			dir, filematch := filepath.Split(includePattern)
			dir = filepath.Join(rootPath, dir)
			exists, err := FileExists(dir)
			if err != nil {
				errChan <- err
			}
			if !exists {
				continue
			}

			if ctx.Err() != nil {
				errChan <- errors.New("archive creation operation canceled")
			}

			var walk filepath.WalkFunc

			if filematch == "**" {
				walk = func(path string, info os.FileInfo, err error) error {
					outputChan <- ArchiveContentFile{path, info}
					return nil
				}
				grip.Warning(filepath.Walk(dir, walk))
			} else if strings.Contains(filematch, "**") {
				globSuffix := filematch[2:]
				walk = func(path string, info os.FileInfo, err error) error {
					if strings.HasSuffix(filepath.Base(path), globSuffix) {
						outputChan <- ArchiveContentFile{path, info}
					}
					return nil
				}
				grip.Warning(filepath.Walk(dir, walk))
			} else {
				walk = func(path string, info os.FileInfo, err error) error {
					a, b := filepath.Split(path)
					if filepath.Clean(a) == filepath.Clean(dir) {
						match, err := filepath.Match(filematch, b)
						if err != nil {
							errChan <- err
						}
						if match {
							outputChan <- ArchiveContentFile{path, info}
						}
					}
					return nil
				}
				grip.Warning(filepath.Walk(rootPath, walk))
			}
		}
	}()

	return outputChan, errChan
}
