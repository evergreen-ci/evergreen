package pail

import (
	"context"
	"crypto"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChecksum(t *testing.T) {
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
				out, err := checksum(hash.New(), "")
				assert.Error(t, err)
				assert.Zero(t, out)
			})
			t.Run("FileIsNotReadable", func(t *testing.T) {
				out, err := checksum(hash.New(), "/root/.bashrc")
				assert.Error(t, err)
				assert.Zero(t, out)
			})
			t.Run("FileIsDirectory", func(t *testing.T) {
				out, err := checksum(hash.New(), filepath.Dir(file))
				assert.Error(t, err)
				assert.Zero(t, out)
			})
			t.Run("FileExists", func(t *testing.T) {
				out, err := checksum(hash.New(), file)
				assert.NoError(t, err)
				assert.NotZero(t, out)
			})
		})
	}
	t.Run("NilHash", func(t *testing.T) {
		assert.Panics(t, func() {
			out, err := checksum(nil, file)
			assert.Error(t, err)
			assert.Zero(t, out)
		})
	})
	t.Run("ChecksumFrontends", func(t *testing.T) {
		out, err := md5sum(file)
		assert.NoError(t, err)
		assert.NotZero(t, out)

		out, err = sha1sum(file)
		assert.NoError(t, err)
		assert.NotZero(t, out)
	})
}

func TestWalkTree(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	ctx := context.Background()

	t.Run("CanceledContext", func(t *testing.T) {
		tctx, cancel := context.WithCancel(ctx)
		cancel()
		out, err := walkLocalTree(tctx, filepath.Dir(file))
		assert.Error(t, err)
		assert.Nil(t, out)
	})
	t.Run("MissingPath", func(t *testing.T) {
		out, err := walkLocalTree(ctx, "")
		assert.NoError(t, err)
		assert.Nil(t, out)
	})
	t.Run("WorkingExample", func(t *testing.T) {
		out, err := walkLocalTree(ctx, filepath.Dir(file))
		assert.NoError(t, err)
		assert.NotNil(t, out)
	})
}
