package util

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// TarContentsFile represents a tar file on disk.
type TarContentsFile struct {
	path string
	info os.FileInfo
}

// BuildArchive reads the rootPath directory into the tar.Writer,
// taking included and excluded strings into account.
// Returns the number of files that were added to the archive
func BuildArchive(ctx context.Context, tarWriter *tar.Writer, rootPath string, includes []string,
	excludes []string, logger grip.Journaler) (int, error) {

	pathsToAdd := make(chan TarContentsFile)
	done := make(chan bool)
	errChan := make(chan error)
	numFilesArchived := 0
	go func(outputChan chan TarContentsFile) {
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
					outputChan <- TarContentsFile{path, info}
					return nil
				}
				logger.Warning(filepath.Walk(dir, walk))
			} else if strings.Contains(filematch, "**") {
				globSuffix := filematch[2:]
				walk = func(path string, info os.FileInfo, err error) error {
					if strings.HasSuffix(filepath.Base(path), globSuffix) {
						outputChan <- TarContentsFile{path, info}
					}
					return nil
				}
				logger.Warning(filepath.Walk(dir, walk))
			} else {
				walk = func(path string, info os.FileInfo, err error) error {
					a, b := filepath.Split(path)
					if filepath.Clean(a) == filepath.Clean(dir) {
						match, err := filepath.Match(filematch, b)
						if err != nil {
							errChan <- err
						}
						if match {
							outputChan <- TarContentsFile{path, info}
						}
					}
					return nil
				}
				logger.Warning(filepath.Walk(rootPath, walk))
			}
		}
		close(outputChan)
	}(pathsToAdd)

	go func(inputChan chan TarContentsFile) {
		processed := map[string]bool{}
	FileChanLoop:
		for file := range inputChan {
			if ctx.Err() != nil {
				return
			}

			var intarball string
			// Tarring symlinks doesn't work reliably right now, so if the file is
			// a symlink, leave intarball path intact but write from the file
			// underlying the symlink.
			if file.info.Mode()&os.ModeSymlink > 0 {
				symlinkPath, err := filepath.EvalSymlinks(file.path)
				if err != nil {
					logger.Warningf("Could not follow symlink %s, ignoring", file.path)
					continue
				} else {
					logger.Infof("Following symlink in %s, got: %s", file.path, symlinkPath)
					symlinkFileInfo, err := os.Stat(symlinkPath)
					if err != nil {
						logger.Warningf("Failed to get underlying file '%s' for symlink '%s', ignoring", symlinkPath, file.path)
						continue
					}

					intarball = strings.Replace(file.path, "\\", "/", -1)
					file.path = symlinkPath
					file.info = symlinkFileInfo
				}
			} else {
				intarball = strings.Replace(file.path, "\\", "/", -1)
			}
			rootPathPrefix := strings.Replace(rootPath, "\\", "/", -1)
			intarball = strings.Replace(intarball, "\\", "/", -1)
			intarball = strings.Replace(intarball, rootPathPrefix, "", 1)
			intarball = filepath.Clean(intarball)
			intarball = strings.Replace(intarball, "\\", "/", -1)

			//strip any leading slash from the tarball header path
			intarball = strings.TrimLeft(intarball, "/")

			logger.Infoln("adding to tarball:", intarball)
			if _, hasKey := processed[intarball]; hasKey {
				continue
			} else {
				processed[intarball] = true
			}
			if file.info.IsDir() {
				continue
			}

			_, fileName := filepath.Split(file.path)
			for _, ignore := range excludes {
				if match, _ := filepath.Match(ignore, fileName); match {
					continue FileChanLoop
				}
			}

			hdr := new(tar.Header)
			hdr.Name = strings.TrimPrefix(intarball, rootPathPrefix)
			hdr.Mode = int64(file.info.Mode())
			hdr.Size = file.info.Size()
			hdr.ModTime = file.info.ModTime()

			numFilesArchived++
			err := tarWriter.WriteHeader(hdr)
			if err != nil {
				errChan <- errors.Wrapf(err, "Error writing header for %v", intarball)
				return
			}

			in, err := os.Open(file.path)
			if err != nil {
				errChan <- errors.Wrapf(err, "Error opening %v", file.path)
				return
			}

			amountWrote, err := io.Copy(tarWriter, in)
			if err != nil {
				logger.Debug(in.Close())
				errChan <- errors.Wrapf(err, "Error writing into tar for %v", file.path)
				return
			}

			if amountWrote != hdr.Size {
				logger.Debug(in.Close())
				errChan <- errors.Errorf(`Error writing to archive for %v:
					header size %v but wrote %v`,
					intarball, hdr.Size, amountWrote)
				return
			}
			logger.Debug(in.Close())
			logger.Warning(tarWriter.Flush())
		}
		done <- true
	}(pathsToAdd)

	select {
	case <-ctx.Done():
		return numFilesArchived, errors.New("archive creation operation canceled")
	case <-done:
		return numFilesArchived, nil
	case err := <-errChan:
		return numFilesArchived, err
	}
}

// Extract unpacks the tar.Reader into rootPath.
func Extract(ctx context.Context, tarReader *tar.Reader, rootPath string) error {
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			return nil //reached end of archive, we are done.
		}
		if err != nil {
			return errors.WithStack(err)
		}
		if ctx.Err() != nil {
			return errors.New("extraction operation canceled")
		}

		if hdr.Typeflag == tar.TypeDir {
			// this tar entry is a directory - need to mkdir it
			localDir := fmt.Sprintf("%v/%v", rootPath, hdr.Name)
			err = os.MkdirAll(localDir, 0755)
			if err != nil {
				return errors.WithStack(err)
			}
		} else if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
			// this tar entry is a regular file (not a dir or link)
			// first, ensure the file's parent directory exists
			localFile := fmt.Sprintf("%v/%v", rootPath, hdr.Name)
			dir := filepath.Dir(localFile)
			err = os.MkdirAll(dir, 0755)
			if err != nil {
				return errors.WithStack(err)
			}

			// Now create the file itself, and write in the contents.

			// Not using 'defer f.Close()' because this is in a loop,
			// and we don't want to wait for the whole archive to finish to
			// close the files - so each is closed explicitly.

			f, err := os.Create(localFile)
			if err != nil {
				return errors.WithStack(err)
			}

			_, err = io.Copy(f, tarReader)
			if err != nil {
				grip.CatchError(f.Close())
				return errors.WithStack(err)
			}

			// File's permissions should match what was in the archive
			err = os.Chmod(f.Name(), os.FileMode(int32(hdr.Mode)))
			if err != nil {
				grip.CatchError(f.Close())
				return errors.WithStack(err)
			}
			grip.CatchError(f.Close())
		} else {
			return errors.New("Unknown file type in archive.")
		}
	}
}

// TarGzReader returns a file, gzip reader, and tar reader for the given path.
// The tar reader wraps the gzip reader, which wraps the file.
func TarGzReader(path string) (f, gz io.ReadCloser, tarReader *tar.Reader, err error) {
	f, err = os.Open(path)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	gz, err = gzip.NewReader(f)
	if err != nil {
		defer f.Close()
		return nil, nil, nil, errors.WithStack(err)
	}
	tarReader = tar.NewReader(gz)
	return f, gz, tarReader, nil
}

// TarGzWriter returns a file, gzip writer, and tarWriter for the path.
// The tar writer wraps the gzip writer, which wraps the file.
func TarGzWriter(path string) (f, gz io.WriteCloser, tarWriter *tar.Writer, err error) {
	f, err = os.Create(path)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	gz = gzip.NewWriter(f)
	tarWriter = tar.NewWriter(gz)
	return f, gz, tarWriter, nil
}
