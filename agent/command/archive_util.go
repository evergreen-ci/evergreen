package command

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/utility"
	"github.com/klauspost/pgzip"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// buildArchive reads the rootPath directory into the tar.Writer,
// taking included and excluded strings into account.
// Returns the number of files that were added to the archive
func buildArchive(ctx context.Context, tarWriter *tar.Writer, rootPath string, pathsToAdd []archiveContentFile,
	excludes []string, logger grip.Journaler) (int, error) {

	numFilesArchived := 0
	processed := map[string]bool{}
	logger.Infof("Beginning to build archive.")
FileLoop:
	for _, file := range pathsToAdd {
		if err := ctx.Err(); err != nil {
			return numFilesArchived, errors.Wrap(err, "building archive was canceled")
		}

		var intarball string
		// Tarring symlinks doesn't work reliably right now, so if the file is
		// a symlink, leave intarball path intact but write from the file
		// underlying the symlink.
		if file.info.Mode()&os.ModeSymlink > 0 {
			symlinkPath, err := filepath.EvalSymlinks(file.path)
			if err != nil {
				logger.Warningf("Could not follow symlink '%s', ignoring.", file.path)
				continue
			} else {
				logger.Infof("Following symlink '%s', got path '%s'.", file.path, symlinkPath)
				symlinkFileInfo, err := os.Stat(symlinkPath)
				if err != nil {
					logger.Warningf("Failed to get underlying file for symlink '%s', ignoring.", file.path)
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

		logger.Infof("Adding file to tarball: '%s'.", intarball)
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
				continue FileLoop
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
			return numFilesArchived, errors.Wrapf(err, "writing tarball header for file '%s'", intarball)
		}

		in, err := os.Open(file.path)
		if err != nil {
			return numFilesArchived, errors.Wrapf(err, "opening file '%s'", file.path)
		}
		amountWrote, err := io.Copy(tarWriter, in)
		if err != nil {
			logger.Debug(errors.Wrapf(in.Close(), "closing file '%s'", file.path))
			return numFilesArchived, errors.Wrapf(err, "copying file '%s' into tarball", file.path)
		}

		if amountWrote != hdr.Size {
			logger.Debug(errors.Wrapf(in.Close(), "closing file '%s'", file.path))
			return numFilesArchived, errors.Errorf("tarball header size is %d but actually wrote %d", hdr.Size, amountWrote)
		}

		logger.Debug(errors.Wrapf(in.Close(), "closing file '%s'", file.path))
		logger.Warning(errors.Wrap(tarWriter.Flush(), "flushing tar writer"))
	}

	return numFilesArchived, nil
}

func extractTarball(ctx context.Context, reader io.Reader, rootPath string, excludes []string) error {
	// wrap the reader in a gzip reader and a tar reader
	gzipReader, err := pgzip.NewReader(reader)
	if err != nil {
		return errors.Wrap(err, "creating gzip reader")
	}

	tarReader := tar.NewReader(gzipReader)
	err = extractTarArchive(ctx, tarReader, rootPath, excludes)
	if err != nil {
		return errors.Wrapf(err, "extracting path '%s'", rootPath)
	}

	return nil
}

// extractTarArchive unpacks the tar.Reader into rootPath.
func extractTarArchive(ctx context.Context, tarReader *tar.Reader, rootPath string, excludes []string) error {
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			return nil //reached end of archive, we are done.
		}
		if err != nil {
			return errors.WithStack(err)
		}
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "extraction operation canceled")
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
				grip.Error(errors.Wrapf(f.Close(), "closing file '%s'", localFile))
				return errors.Wrap(err, "copying tar contents to local file '%s'")
			}

			// File's permissions should match what was in the archive
			if err = os.Chmod(f.Name(), os.FileMode(int32(hdr.Mode))); err != nil {
				grip.Error(errors.Wrapf(f.Close(), "closing file '%s'", localFile))
				return errors.Wrapf(err, "changing file '%s' mode to %d", f.Name(), hdr.Mode)
			}

			grip.Error(errors.Wrapf(f.Close(), "closing file '%s'", localFile))
		} else {
			return errors.Errorf("unknown file type '%c' in archive", hdr.Typeflag)
		}
	}
}

// tarGzReader returns a file, gzip reader, and tar reader for the given path.
// The tar reader wraps the gzip reader, which wraps the file.
func tarGzReader(path string) (f, gz io.ReadCloser, tarReader *tar.Reader, err error) {
	f, err = os.Open(path)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "opening file '%s'", path)
	}
	// kim: NOTE: only use pgzip if it's larger than 1 MB because it performs
	// worse on small archives and many archives in Evergreen are small.
	gz, err = pgzip.NewReader(f)
	if err != nil {
		defer f.Close()
		return nil, nil, nil, errors.Wrap(err, "initializing gzip reader")
	}
	tarReader = tar.NewReader(gz)
	return f, gz, tarReader, nil
}

// tarGzWriter returns a file, gzip writer, and tarWriter for the path.
// The tar writer wraps the gzip writer, which wraps the file.
func tarGzWriter(path string, useParallelGzip bool) (f, gz io.WriteCloser, tarWriter *tar.Writer, err error) {
	f, err = os.Create(path)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "creating file '%s'", path)
	}
	if useParallelGzip {
		gz = pgzip.NewWriter(f)
	} else {
		gz = gzip.NewWriter(f)
	}
	tarWriter = tar.NewWriter(gz)
	return f, gz, tarWriter, nil
}

// archiveContentFile represents a tar file on disk.
type archiveContentFile struct {
	path string
	info os.FileInfo
	err  error
}

// findContentsToArchive finds all files starting from the rootPath with the
// given inclusion and exclusion patterns.
func findContentsToArchive(ctx context.Context, rootPath string, includes, excludes []string) (files []archiveContentFile, totalSize int, err error) {
	out := []archiveContentFile{}
	catcher := grip.NewBasicCatcher()
	archiveContents, totalSize, err := streamArchiveContents(ctx, rootPath, includes, excludes)
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting archive contents")
	}
	for _, fn := range archiveContents {
		if fn.err != nil {
			catcher.Add(fn.err)
			continue
		}

		out = append(out, fn)
	}

	if catcher.HasErrors() {
		return nil, 0, catcher.Resolve()
	}

	return out, totalSize, nil
}

func streamArchiveContents(ctx context.Context, rootPath string, includes, excludes []string) (files []archiveContentFile, totalSize int, err error) {
	archiveContents := []archiveContentFile{}
	seen := map[string]archiveContentFile{}
	catcher := grip.NewCatcher()

	addUniqueFile := func(path string, info fs.FileInfo) {
		if _, ok := seen[path]; ok {
			return
		}

		acf := archiveContentFile{path: path, info: info}
		seen[path] = acf
		archiveContents = append(archiveContents, acf)
		if info.Mode().IsRegular() {
			totalSize += int(info.Size())
		}
	}

	for _, includePattern := range includes {
		dir, filematch := filepath.Split(includePattern)
		dir = filepath.Join(rootPath, dir)

		if !utility.FileExists(dir) {
			continue
		}

		if err := ctx.Err(); err != nil {
			return nil, 0, errors.Wrapf(err, "canceled while streaming archive for include pattern '%s'", includePattern)
		}

		var walk filepath.WalkFunc

		if filematch == "**" {
			walk = func(path string, info os.FileInfo, err error) error {
				for _, ignore := range excludes {
					if match, _ := filepath.Match(ignore, path); match {
						return nil
					}
				}

				addUniqueFile(path, info)

				return nil
			}
			catcher.Wrapf(filepath.Walk(dir, walk), "matching files included in filter '%s' for path '%s'", filematch, dir)
		} else if strings.Contains(filematch, "**") {
			globSuffix := filematch[2:]
			walk = func(path string, info os.FileInfo, err error) error {
				if strings.HasSuffix(filepath.Base(path), globSuffix) {
					for _, ignore := range excludes {
						if match, _ := filepath.Match(ignore, path); match {
							return nil
						}
					}

					addUniqueFile(path, info)
				}
				return nil
			}
			catcher.Wrapf(filepath.Walk(dir, walk), "matching files included in filter '%s' for path '%s'", filematch, dir)
		} else {
			walk = func(path string, info os.FileInfo, err error) error {
				a, b := filepath.Split(path)
				if filepath.Clean(a) == filepath.Clean(dir) {
					match, err := filepath.Match(filematch, b)
					if err != nil {
						archiveContents = append(archiveContents, archiveContentFile{err: err})
					}
					if match {
						for _, ignore := range excludes {
							if exmatch, _ := filepath.Match(ignore, path); exmatch {
								return nil
							}
						}

						addUniqueFile(path, info)
					}
				}
				return nil
			}
			catcher.Wrapf(filepath.Walk(rootPath, walk), "matching files included in filter '%s' for patch '%s'", filematch, rootPath)
		}
	}

	return archiveContents, totalSize, catcher.Resolve()
}
