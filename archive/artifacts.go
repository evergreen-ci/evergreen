package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/util"
)

// TarContentsFile represents a tar file on disk.
type TarContentsFile struct {
	path string
	info os.FileInfo
}

// BuildArchive reads the rootPath directory into the tar.Writer,
// taking included and excluded strings into account.
// Returns the number of files that were added to the archive
func BuildArchive(tarWriter *tar.Writer, rootPath string, includes []string,
	excludes []string, log *slogger.Logger) (int, error) {

	pathsToAdd := make(chan TarContentsFile)
	done := make(chan bool)
	errChan := make(chan error)
	numFilesArchived := 0
	go func(outputChan chan TarContentsFile) error {
		for _, includePattern := range includes {
			dir, filematch := filepath.Split(includePattern)
			dir = filepath.Join(rootPath, dir)
			exists, err := util.FileExists(dir)
			if err != nil {
				errChan <- err
			}
			if !exists {
				continue
			}

			var walk filepath.WalkFunc

			if filematch == "**" {
				walk = func(path string, info os.FileInfo, err error) error {
					outputChan <- TarContentsFile{path, info}
					return nil
				}
				filepath.Walk(dir, walk)
			} else if strings.Contains(filematch, "**") {
				globSuffix := filematch[2:]
				walk = func(path string, info os.FileInfo, err error) error {
					if strings.HasSuffix(filepath.Base(path), globSuffix) {
						outputChan <- TarContentsFile{path, info}
					}
					return nil
				}
				filepath.Walk(dir, walk)
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
				filepath.Walk(rootPath, walk)
			}
		}
		close(outputChan)
		return nil
	}(pathsToAdd)

	go func(inputChan chan TarContentsFile) {
		processed := map[string]bool{}
	FileChanLoop:
		for file := range inputChan {
			intarball := ""
			// Tarring symlinks doesn't work reliably right now, so if the file is
			// a symlink, leave intarball path intact but write from the file
			// underlying the symlink.
			if file.info.Mode()&os.ModeSymlink > 0 {
				symlinkPath, err := filepath.EvalSymlinks(file.path)
				if err != nil {
					log.Logf(slogger.WARN, "Could not follow symlink %v, ignoring", file.path)
					continue
				} else {
					log.Logf(slogger.INFO, "Following symlink in %v, got: %v", file.path, symlinkPath)
					symlinkFileInfo, err := os.Stat(symlinkPath)
					if err != nil {
						log.Logf(slogger.WARN, "Failed to get underlying file `%v` for symlink %v, ignoring", symlinkPath, file.path)
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

			log.Logf(slogger.INFO, "Adding to tarball: %s", intarball)
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
				errChan <- fmt.Errorf("Error writing header for %v: %v", intarball, err)
				return
			}

			in, err := os.Open(file.path)
			if err != nil {
				errChan <- fmt.Errorf("Error opening %v: %v", file.path, err)
				return
			}

			amountWrote, err := io.Copy(tarWriter, in)
			if err != nil {
				in.Close()
				errChan <- fmt.Errorf("Error writing into tar for %v: %v", file.path, err)
				return
			}

			if amountWrote != hdr.Size {
				in.Close()
				errChan <- fmt.Errorf(`Error writing to archive for %v:
					header size %v but wrote %v`,
					intarball,
					hdr.Size, amountWrote)
				return
			}
			in.Close()
			tarWriter.Flush()
		}
		done <- true
	}(pathsToAdd)

	select {
	case _ = <-done:
		return numFilesArchived, nil
	case err := <-errChan:
		return numFilesArchived, err
	}
}

// Extract unpacks the tar.Reader into rootPath.
func Extract(tarReader *tar.Reader, rootPath string) error {
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			return nil //reached end of archive, we are done.
		}
		if err != nil {
			return err
		}

		if hdr.Typeflag == tar.TypeDir {
			// this tar entry is a directory - need to mkdir it
			localDir := fmt.Sprintf("%v/%v", rootPath, hdr.Name)
			err = os.MkdirAll(localDir, 0755)
			if err != nil {
				return err
			}
		} else if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
			// this tar entry is a regular file (not a dir or link)
			// first, ensure the file's parent directory exists
			localFile := fmt.Sprintf("%v/%v", rootPath, hdr.Name)
			dir := filepath.Dir(localFile)
			err = os.MkdirAll(dir, 0755)
			if err != nil {
				return err
			}

			// Now create the file itself, and write in the contents.

			// Not using 'defer f.Close()' because this is in a loop,
			// and we don't want to wait for the whole archive to finish to
			// close the files - so each is closed explicitly.

			f, err := os.Create(localFile)
			if err != nil {
				return err
			}

			_, err = io.Copy(f, tarReader)
			if err != nil {
				f.Close()
				return err
			}

			// File's permissions should match what was in the archive
			err = os.Chmod(f.Name(), os.FileMode(int32(hdr.Mode)))
			if err != nil {
				f.Close()
				return err
			}
			f.Close()
		} else {
			return fmt.Errorf("Unknown file type in archive.")
		}
	}
}

// TarGzReader returns a file, gzip reader, and tar reader for the given path.
// The tar reader wraps the gzip reader, which wraps the file.
func TarGzReader(path string) (f, gz io.ReadCloser, tarReader *tar.Reader, err error) {
	f, err = os.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}
	gz, err = gzip.NewReader(f)
	if err != nil {
		defer f.Close()
		return nil, nil, nil, err
	}
	tarReader = tar.NewReader(gz)
	return f, gz, tarReader, nil
}

// TarGzWriter returns a file, gzip writer, and tarWriter for the path.
// The tar writer wraps the gzip writer, which wraps the file.
func TarGzWriter(path string) (f, gz io.WriteCloser, tarWriter *tar.Writer, err error) {
	f, err = os.Create(path)
	if err != nil {
		return nil, nil, nil, err
	}
	gz = gzip.NewWriter(f)
	tarWriter = tar.NewWriter(gz)
	return f, gz, tarWriter, nil
}
