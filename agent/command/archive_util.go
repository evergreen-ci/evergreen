package command

import (
	"archive/tar"
	"compress/gzip"
	"context"
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

// validateRelativePath checks if the filePath is relative to the rootpath.
func validateRelativePath(filePath, rootPath string) error {
	if filepath.IsAbs(filePath) {
		return errors.New("filepath is absolute")
	}
	realPath := filepath.Join(rootPath, filePath)
	resolvedPath, err := filepath.EvalSymlinks(realPath)
	// If the error is a non-existence error, we can ignore it.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrap(err, "evaluating symlinks")
	}
	// If the path was resolved, use the resolved path.
	if err == nil {
		realPath = resolvedPath
	}
	relpath, err := filepath.Rel(rootPath, realPath)
	if err != nil {
		return errors.Wrap(err, "getting relative path")
	}
	if strings.Contains(relpath, "..") {
		return errors.New("relative path starts with '..'")
	}
	return nil
}

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
	err = extractTarballArchive(ctx, tarReader, rootPath, excludes)
	if err != nil {
		return errors.Wrapf(err, "extracting path '%s'", rootPath)
	}

	return nil
}

// extractTarballArchive unpacks the tar.Reader into rootPath.
func extractTarballArchive(ctx context.Context, tarReader *tar.Reader, rootPath string, excludes []string) error {
	// Link files and symlink files are extracted after all other files are extracted.
	linkFiles := []func() error{}
tarReaderLoop:
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			// This means we've reached the end of the archive file and
			// we can do the link and symlink files.
			for _, f := range linkFiles {
				if err := f(); err != nil {
					return errors.Wrap(err, "")
				}
			}
			return nil
		}
		if err != nil {
			// This is an unexpected error.
			return errors.Wrap(errors.WithStack(err), "getting next tar entry")
		}
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "extraction operation canceled")
		}

		name := hdr.Name
		linkname := hdr.Linkname
		if err := validateRelativePath(name, rootPath); err != nil {
			return errors.Wrapf(err, "artifact path name '%s' should be relative to the root path", name)
		}
		if linkname != "" {
			if err := validateRelativePath(linkname, rootPath); err != nil {
				return errors.Wrapf(err, "artifact path link name '%s' should be relative to the root path", name)
			}
		}

		namePath := filepath.Join(rootPath, name)
		linkNamePath := filepath.Join(rootPath, linkname)

		for _, ignore := range excludes {
			if match, _ := filepath.Match(ignore, name); match {
				continue tarReaderLoop
			}
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			// Tar entry for a directory.
			if err = os.MkdirAll(namePath, 0755); err != nil {
				return errors.WithStack(err)
			}
		case tar.TypeLink:
			// Tar entry for a hard link.
			linkFiles = append(linkFiles, func() error {
				return os.Link(linkNamePath, namePath)
			})
		case tar.TypeSymlink:
			// Tar entry for a symbolic link.
			linkFiles = append(linkFiles, func() error {
				return os.Symlink(linkNamePath, namePath)
			})
		case tar.TypeReg, tar.TypeRegA:
			// Tar entry for a regular file.
			// First, ensure the file's parent directory exists.
			if err = os.MkdirAll(filepath.Dir(namePath), 0755); err != nil {
				return errors.WithStack(err)
			}

			err := writeFileWithContentsAndPermission(namePath, tarReader, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
		default:
			return errors.Errorf("unknown file type '%c' in archive", hdr.Typeflag)
		}
	}
}

func writeFileWithContentsAndPermission(path string, contents io.Reader, mode fs.FileMode) error {
	f, err := os.Create(path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		grip.Error(errors.Wrapf(f.Close(), "closing file '%s'", path))
	}()

	if _, err = io.Copy(f, contents); err != nil {
		return errors.Wrap(err, "copying tar contents to local file")
	}

	return errors.Wrapf(os.Chmod(f.Name(), mode), "changing file '%s' mode to %d", f.Name(), mode)
}

// tarGzReader returns a file, gzip reader, and tar reader for the given path.
// The tar reader wraps the gzip reader, which wraps the file.
func tarGzReader(path string) (f, gz io.ReadCloser, tarReader *tar.Reader, err error) {
	f, err = os.Open(path)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "opening file '%s'", path)
	}
	gz, err = pgzip.NewReader(f)
	if err != nil {
		defer f.Close()
		return nil, nil, nil, errors.Wrap(err, "initializing gzip reader")
	}
	tarReader = tar.NewReader(gz)
	return f, gz, tarReader, nil
}

// tarGzWriter returns a file, gzip writer, and tarWriter for the path.
// The tar writer wraps the gzip writer, which wraps the file. If
// useParallelGzip is true, then it will use the parallel gzip algorithm;
// otherwise, it'll use the standard single-threaded gzip algorithm.
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
	archiveContents, totalSize, err := findArchiveContents(ctx, rootPath, includes, excludes)
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

// findArchiveContents returns files to be archived starting at the rootPath. It
// matches all files that match any of the include filters and are not excluded
// by one of the exclude filters. At least one include filter must be given for
// this to return any files.
func findArchiveContents(ctx context.Context, rootPath string, includes, excludes []string) (files []archiveContentFile, totalSize int, err error) {
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
				if err != nil {
					return err
				}

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
				if err != nil {
					return err
				}

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
				if err != nil {
					return err
				}

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
