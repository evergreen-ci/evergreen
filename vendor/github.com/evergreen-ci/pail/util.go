package pail

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

func checksum(hash hash.Hash, path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", errors.Wrapf(err, "file '%s' does not exist", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "problem opening '%s'", path)
	}
	defer f.Close()

	_, err = io.Copy(hash, f)
	if err != nil {
		return "", errors.Wrap(err, "problem hashing contents")
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func md5sum(path string) (string, error) {
	out, err := checksum(md5.New(), path)
	return out, errors.WithStack(err)
}

func sha1sum(path string) (string, error) {
	out, err := checksum(sha1.New(), path)
	return out, errors.WithStack(err)
}

func walkLocalTree(ctx context.Context, prefix string) ([]string, error) {
	var out []string
	err := filepath.Walk(prefix, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(prefix, path)
		if err != nil {
			return errors.Wrap(err, "problem getting relative path")
		}
		out = append(out, rel)
		return nil
	})

	if err != nil {
		return nil, errors.Wrap(err, "problem finding files")
	}

	return out, nil
}
