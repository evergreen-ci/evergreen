package options

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// WriteFile represents the information necessary to write to a file.
type WriteFile struct {
	Path string `json:"path"`
	// File content can come from either Content or Reader, but not both.
	// Content should only be used if the entire file's contents can be held in
	// memory.
	Content []byte      `json:"content"`
	Reader  io.Reader   `json:"-"`
	Append  bool        `json:"append"`
	Perm    os.FileMode `json:"perm"`
}

// validateContent ensures that there is at most one source of content for
// the file.
func (info *WriteFile) validateContent() error {
	if len(info.Content) > 0 && info.Reader != nil {
		return errors.New("cannot have both data and reader set as file content")
	}
	// If neither is set, ensure that Content is empty rather than nil to
	// prevent potential writes with a nil slice.
	if len(info.Content) == 0 && info.Reader == nil {
		info.Content = []byte{}
	}
	return nil
}

// Validate ensures that all the parameters to write to a file are valid and sets
// default permissions if necessary.
func (info *WriteFile) Validate() error {
	catcher := grip.NewBasicCatcher()
	if info.Path == "" {
		catcher.New("path to file must be specified")
	}

	if info.Perm == 0 {
		info.Perm = 0666
	}

	catcher.Add(info.validateContent())

	return catcher.Resolve()
}

// DoWrite writes the data to the given path, creating the directory hierarchy as
// needed and the file if it does not exist yet.
func (info *WriteFile) DoWrite() error {
	if err := makeEnclosingDirectories(filepath.Dir(info.Path)); err != nil {
		return errors.Wrap(err, "problem making enclosing directories")
	}

	openFlags := os.O_RDWR | os.O_CREATE
	if info.Append {
		openFlags |= os.O_APPEND
	} else {
		openFlags |= os.O_TRUNC
	}

	file, err := os.OpenFile(info.Path, openFlags, 0666)
	if err != nil {
		return errors.Wrapf(err, "error opening file %s", info.Path)
	}

	catcher := grip.NewBasicCatcher()

	reader, err := info.ContentReader()
	if err != nil {
		catcher.Wrap(file.Close(), "error closing file")
		catcher.Wrap(err, "error getting file content as bytes")
		return catcher.Resolve()
	}

	bufReader := bufio.NewReader(reader)
	if _, err = io.Copy(file, bufReader); err != nil {
		catcher.Wrap(file.Close(), "error closing file")
		catcher.Wrap(err, "error writing content to file")
		return catcher.Resolve()
	}

	return errors.Wrap(file.Close(), "error closing file")
}

// WriteBufferedContent writes the content to a file by repeatedly calling
// doWrite with a buffered portion of the content. doWrite processes the
// WriteFile containing the next content to write to the file.
func (info *WriteFile) WriteBufferedContent(doWrite func(bufInfo WriteFile) error) error {
	if err := info.validateContent(); err != nil {
		return errors.Wrap(err, "could not validate file content source")
	}
	didWrite := false
	for buf, err := info.contentBytes(); len(buf) != 0; buf, err = info.contentBytes() {
		if err != nil && err != io.EOF {
			return errors.Wrap(err, "error getting content bytes")
		}

		bufInfo := *info
		bufInfo.Content = buf
		if didWrite {
			bufInfo.Append = true
		}

		if err := doWrite(bufInfo); err != nil {
			return errors.Wrap(err, "could not write info")
		}

		didWrite = true

		if err == io.EOF {
			break
		}
	}

	if didWrite {
		return nil
	}

	return errors.Wrap(doWrite(*info), "could not write info")

}

// SetPerm sets the file permissions on the file. This should be called after
// DoWrite. If no file exists at (WriteFile).Path, it will error.
func (info *WriteFile) SetPerm() error {
	return errors.Wrap(os.Chmod(info.Path, info.Perm), "error setting permissions")
}

// contentBytes returns the contents to be written to the file as a byte slice.
// and will return io.EOF when all the file content has been received. Callers
// should process the byte slice before checking for the io.EOF condition.
func (info *WriteFile) contentBytes() ([]byte, error) {
	if err := info.validateContent(); err != nil {
		return nil, errors.Wrap(err, "could not validate file content source")
	}

	if info.Reader != nil {
		const mb = 1024 * 1024
		buf := make([]byte, mb)
		n, err := info.Reader.Read(buf)
		return buf[:n], err
	}

	return info.Content, io.EOF
}

// ContentReader returns the contents to be written to the file as an io.Reader.
func (info *WriteFile) ContentReader() (io.Reader, error) {
	if err := info.validateContent(); err != nil {
		return nil, errors.Wrap(err, "could not validate file content source")
	}

	if info.Reader != nil {
		return info.Reader, nil
	}

	info.Reader = bytes.NewBuffer(info.Content)
	info.Content = nil

	return info.Reader, nil
}
