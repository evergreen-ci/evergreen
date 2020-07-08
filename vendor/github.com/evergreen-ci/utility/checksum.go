package utility

import (
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"hash"
	"io"
	"os"

	"github.com/pkg/errors"
)

// ChecksumFile returns the checksum of a file given by path using the given
// hash.
func ChecksumFile(hash hash.Hash, path string) (string, error) {
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

// MD5SumFile is the same as ChecksumFile using MD5 as the checksum.
func MD5SumFile(path string) (string, error) {
	out, err := ChecksumFile(md5.New(), path)
	return out, errors.WithStack(err)
}

// SHA1SumFile is the same as ChecksumFile using SHA1 as the checksum.
func SHA1SumFile(path string) (string, error) {
	out, err := ChecksumFile(sha1.New(), path)
	return out, errors.WithStack(err)
}
