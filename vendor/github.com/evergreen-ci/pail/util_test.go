package pail

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	t.Run("SymLink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("git symlinks do not work on windows")
		}
		vendor, err := walkLocalTree(ctx, "vendor")
		require.NoError(t, err)

		out, err := walkLocalTree(ctx, "testdata")
		require.NoError(t, err)

		fnMap := map[string]bool{}
		for _, fn := range out {
			fnMap[fn] = true
		}
		assert.True(t, fnMap["a_file.txt"])
		assert.True(t, fnMap["z_file.txt"])
		for _, fn := range vendor {
			require.True(t, fnMap[filepath.Join("vendor", fn)])
		}
	})
}

func TestTarFile(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, dir string){
		"CreatesTarWithSingleFile": func(t *testing.T, dir string) {
			fileName := "foo.txt"
			fileContent := "bar"
			require.NoError(t, ioutil.WriteFile(filepath.Join(dir, fileName), []byte(fileContent), 0777))

			b := &bytes.Buffer{}
			tw := tar.NewWriter(b)
			require.NoError(t, tarFile(tw, dir, fileName))

			tr := tar.NewReader(b)
			header, err := tr.Next()
			require.NoError(t, err)
			assert.Equal(t, fileName, header.Name)
			assert.EqualValues(t, tar.TypeReg, header.Typeflag)
			checkContent := &bytes.Buffer{}
			_, err = io.Copy(checkContent, tr)
			require.NoError(t, err)
			assert.Equal(t, fileContent, checkContent.String())

			_, err = tr.Next()
			assert.Equal(t, io.EOF, err)
		},
		"CreatesTarWithDirectory": func(t *testing.T, dir string) {
			subDirName := "foo"
			absPath := filepath.Join(dir, subDirName)
			require.NoError(t, os.Mkdir(absPath, 0777))

			b := &bytes.Buffer{}
			tw := tar.NewWriter(b)
			require.NoError(t, tarFile(tw, dir, subDirName))

			tr := tar.NewReader(b)
			header, err := tr.Next()
			require.NoError(t, err)
			assert.Equal(t, subDirName+"/", header.Name)
			assert.EqualValues(t, tar.TypeDir, header.Typeflag)
		},
		"CreatesTarWithFilesInSubdirectory": func(t *testing.T, dir string) {
			relFilePath := filepath.Join("foo", "bar.txt")
			absPath := filepath.Join(dir, relFilePath)
			require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0777))
			fileContent := []byte("bat")
			require.NoError(t, ioutil.WriteFile(absPath, fileContent, 0777))

			b := &bytes.Buffer{}
			tw := tar.NewWriter(b)
			require.NoError(t, tarFile(tw, dir, relFilePath))

			tr := tar.NewReader(b)
			header, err := tr.Next()
			require.NoError(t, err)
			assert.Equal(t, filepath.ToSlash(relFilePath), header.Name)
			assert.EqualValues(t, tar.TypeReg, header.Typeflag)
			checkContent, err := ioutil.ReadAll(tr)
			require.NoError(t, err)
			assert.Equal(t, fileContent, checkContent)
		},
		"FailsForFileNotWithinBaseDirectory": func(t *testing.T, dir string) {
			tmpFile, err := ioutil.TempFile("", "outside_tar_dir")
			require.NoError(t, err)
			require.NoError(t, tmpFile.Close())

			b := &bytes.Buffer{}
			tw := tar.NewWriter(b)
			assert.Error(t, tarFile(tw, dir, tmpFile.Name()))
		},
		"FailsForNonexistentFile": func(t *testing.T, dir string) {
			b := &bytes.Buffer{}
			tw := tar.NewWriter(b)
			assert.Error(t, tarFile(tw, dir, "nonexistent_file"))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tmpDir, err := ioutil.TempDir("", "tar_file_test")
			require.NoError(t, err)
			testCase(t, tmpDir)
		})
	}
}
