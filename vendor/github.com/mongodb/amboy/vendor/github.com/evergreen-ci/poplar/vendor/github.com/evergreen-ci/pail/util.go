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
	"regexp"

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
	if ctx.Err() != nil {
		return nil, errors.New("operation canceled")
	}

	return out, nil
}

func removePrefix(ctx context.Context, prefix string, b Bucket) error {
	iter, err := b.List(ctx, prefix)
	if err != nil {
		return errors.Wrapf(err, "failed to delete any objects with prefix '%s' for deletion", prefix)
	}

	keys := []string{}
	for iter.Next(ctx) {
		keys = append(keys, iter.Item().Name())
	}
	return errors.Wrapf(b.RemoveMany(ctx, keys...), "failed to delete some objects with prefix '%s'", prefix)
}

func removeMatching(ctx context.Context, expression string, b Bucket) error {
	regex, err := regexp.Compile(expression)
	if err != nil {
		return errors.Wrapf(err, "invalid regular expression '%s'", expression)
	}
	iter, err := b.List(ctx, "")
	if err != nil {
		return errors.Wrapf(err, "failed to delete any objects matching '%s'", expression)
	}

	keys := []string{}
	for iter.Next(ctx) {
		key := iter.Item().Name()
		if regex.MatchString(key) {
			keys = append(keys, key)
		}
	}
	return errors.Wrapf(b.RemoveMany(ctx, keys...), "failed to delete some objects matching '%s'", expression)
}

func consistentJoin(prefix, key string) string {
	if prefix != "" {
		return prefix + "/" + key
	}
	return key
}
