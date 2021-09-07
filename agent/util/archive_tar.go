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

// BuildArchive reads the rootPath directory into the tar.Writer,
// taking included and excluded strings into account.
// Returns the number of files that were added to the archive
func BuildArchive(ctx context.Context, tarWriter *tar.Writer, rootPath string, includes []string,
	excludes []string, logger grip.Journaler) (int, error) {
	pathsToAdd, err := streamArchiveContents(ctx, rootPath, includes, []string{})
	if err != nil {
		return 0, errors.Wrap(err, "problem getting archive contents")
	}

	numFilesArchived := 0
	processed := map[string]bool{}
	logger.Infof("beginning to build archive")
FileLoop:
	for _, file := range pathsToAdd {
		if ctx.Err() != nil {
			return numFilesArchived, errors.Wrap(ctx.Err(), "timeout when building archive")
		}

		var intarball string
		// Tarring symlinks doesn't work reliably right now, so if the file is
		// a symlink, leave intarball path intact but write from the file
		// underlying the symlink.
		if file.Info.Mode()&os.ModeSymlink > 0 {
			symlinkPath, err := filepath.EvalSymlinks(file.Path)
			if err != nil {
				logger.Warningf("Could not follow symlink %s, ignoring", file.Path)
				continue
			} else {
				logger.Infof("Following symlink in %s, got: %s", file.Path, symlinkPath)
				symlinkFileInfo, err := os.Stat(symlinkPath)
				if err != nil {
					logger.Warningf("Failed to get underlying file '%s' for symlink '%s', ignoring", symlinkPath, file.Path)
					continue
				}

				intarball = strings.Replace(file.Path, "\\", "/", -1)
				file.Path = symlinkPath
				file.Info = symlinkFileInfo
			}
		} else {
			intarball = strings.Replace(file.Path, "\\", "/", -1)
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
		if file.Info.IsDir() {
			continue
		}

		_, fileName := filepath.Split(file.Path)
		for _, ignore := range excludes {
			if match, _ := filepath.Match(ignore, fileName); match {
				continue FileLoop
			}
		}

		hdr := new(tar.Header)
		hdr.Name = strings.TrimPrefix(intarball, rootPathPrefix)
		hdr.Mode = int64(file.Info.Mode())
		hdr.Size = file.Info.Size()
		hdr.ModTime = file.Info.ModTime()

		numFilesArchived++
		err := tarWriter.WriteHeader(hdr)
		if err != nil {
			return numFilesArchived, errors.Wrapf(err, "Error writing header for %s", intarball)
		}

		in, err := os.Open(file.Path)
		if err != nil {
			return numFilesArchived, errors.Wrapf(err, "Error opening %s", file.Path)
		}
		amountWrote, err := io.Copy(tarWriter, in)
		if err != nil {
			logger.Debug(in.Close())
			return numFilesArchived, errors.Wrapf(err, "Error writing into tar for %s", file.Path)
		}

		if amountWrote != hdr.Size {
			logger.Debug(in.Close())
			return numFilesArchived, errors.Errorf("Error writing to archive for %s: header size %d but wrote %d", intarball, hdr.Size, amountWrote)
		}
		logger.Debug(in.Close())
		logger.Warning(tarWriter.Flush())
	}

	return numFilesArchived, nil
}

func ExtractTarball(ctx context.Context, reader io.Reader, rootPath string, excludes []string) error {
	// wrap the reader in a gzip reader and a tar reader
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return errors.Wrap(err, "error creating gzip reader")
	}

	tarReader := tar.NewReader(gzipReader)
	err = extractTarArchive(ctx, tarReader, rootPath, excludes)
	if err != nil {
		return errors.Wrapf(err, "error extracting %s", rootPath)
	}

	return nil
}

// Extract unpacks the tar.Reader into rootPath.
func extractTarArchive(ctx context.Context, tarReader *tar.Reader, rootPath string, excludes []string) error {
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
			if err = os.MkdirAll(localDir, 0755); err != nil {
				return errors.WithStack(err)
			}
		} else if hdr.Typeflag == tar.TypeLink {
			if err = os.Link(hdr.Name, hdr.Linkname); err != nil {
				return errors.WithStack(err)
			}
		} else if hdr.Typeflag == tar.TypeSymlink {
			if err = os.Symlink(hdr.Name, hdr.Linkname); err != nil {
				return errors.WithStack(err)
			}
		} else if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
			// this tar entry is a regular file (not a dir or link)
			// first, ensure the file's parent directory exists
			localFile := fmt.Sprintf("%v/%v", rootPath, hdr.Name)

			for _, ignore := range excludes {
				if match, _ := filepath.Match(ignore, localFile); match {
					continue
				}
			}

			dir := filepath.Dir(localFile)
			if err = os.MkdirAll(dir, 0755); err != nil {
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

			if _, err = io.Copy(f, tarReader); err != nil {
				grip.Error(f.Close())
				return errors.WithStack(err)
			}

			// File's permissions should match what was in the archive
			if err = os.Chmod(f.Name(), os.FileMode(int32(hdr.Mode))); err != nil {
				grip.Error(f.Close())
				return errors.WithStack(err)
			}
			grip.Error(f.Close())
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
