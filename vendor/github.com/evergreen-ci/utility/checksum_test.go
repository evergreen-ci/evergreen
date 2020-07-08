package utility

import (
	"crypto"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChecksumFile(t *testing.T) {
	if usr, _ := user.Current(); usr == nil || usr.Username == "root" {
		t.Skip("test assumes not root")
	}
	_, file, _, _ := runtime.Caller(1)

	for name, hash := range map[string]crypto.Hash{
		"MD5":    crypto.MD5,
		"SHA512": crypto.SHA512,
		"SHA1":   crypto.SHA1,
		"SHA256": crypto.SHA256,
	} {
		t.Run(name, func(t *testing.T) {
			t.Run("NoFile", func(t *testing.T) {
				out, err := ChecksumFile(hash.New(), "")
				assert.Error(t, err)
				assert.Zero(t, out)
			})
			t.Run("FileIsNotReadable", func(t *testing.T) {
				out, err := ChecksumFile(hash.New(), "/root/.bashrc")
				assert.Error(t, err)
				assert.Zero(t, out)
			})
			t.Run("FileIsDirectory", func(t *testing.T) {
				out, err := ChecksumFile(hash.New(), filepath.Dir(file))
				assert.Error(t, err)
				assert.Zero(t, out)
			})
			t.Run("FileExists", func(t *testing.T) {
				out, err := ChecksumFile(hash.New(), file)
				assert.NoError(t, err)
				assert.NotZero(t, out)
			})
		})
	}
	t.Run("NilHash", func(t *testing.T) {
		assert.Panics(t, func() {
			out, err := ChecksumFile(nil, file)
			assert.Error(t, err)
			assert.Zero(t, out)
		})
	})
	t.Run("ChecksumFrontends", func(t *testing.T) {
		out, err := MD5SumFile(file)
		assert.NoError(t, err)
		assert.NotZero(t, out)

		out, err = SHA1SumFile(file)
		assert.NoError(t, err)
		assert.NotZero(t, out)
	})
}
