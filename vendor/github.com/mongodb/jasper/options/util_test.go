package options

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sliceContains(group []string, name string) bool {
	for _, g := range group {
		if name == g {
			return true
		}
	}

	return false
}

func TestStringMembership(t *testing.T) {
	cases := []struct {
		id      string
		group   []string
		name    string
		outcome bool
	}{
		{
			id:      "EmptySet",
			group:   []string{},
			name:    "anything",
			outcome: false,
		},
		{
			id:      "ZeroArguments",
			outcome: false,
		},
		{
			id:      "OneExists",
			group:   []string{"a"},
			name:    "a",
			outcome: true,
		},
		{
			id:      "OneOfMany",
			group:   []string{"a", "a", "a"},
			name:    "a",
			outcome: true,
		},
		{
			id:      "OneOfManyDifferentSet",
			group:   []string{"a", "b", "c"},
			name:    "c",
			outcome: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.id, func(t *testing.T) {
			assert.Equal(t, testCase.outcome, sliceContains(testCase.group, testCase.name))
		})
	}
}

func TestMakeEnclosingDirectories(t *testing.T) {
	path := "foo"
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err))
	assert.NoError(t, makeEnclosingDirectories(path))
	defer os.RemoveAll(path)

	_, path, _, ok := runtime.Caller(0)
	require.True(t, ok)
	info, err := os.Stat(path)
	require.False(t, os.IsNotExist(err))
	require.False(t, info.IsDir())
	assert.Error(t, makeEnclosingDirectories(path))
}

func TestWriteFile(t *testing.T) {
	for testName, testCase := range map[string]struct {
		content    string
		path       string
		shouldPass bool
	}{
		"FailsForInsufficientMkdirPermissions": {
			content:    "foo",
			path:       "/bar",
			shouldPass: false,
		},
		"FailsForInsufficientFileWritePermissions": {
			content:    "foo",
			path:       "/etc/hosts",
			shouldPass: false,
		},
		"FailsForInsufficientFileOpenPermissions": {
			content:    "foo",
			path:       "/etc/whatever",
			shouldPass: false,
		},
		"WriteToFileSucceeds": {
			content:    "foo",
			path:       "/dev/null",
			shouldPass: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			if os.Geteuid() == 0 {
				t.Skip("cannot test download permissions as root")
			} else if runtime.GOOS == "windows" {
				t.Skip("cannot run file write tests on windows")
			}
			err := writeFile(bytes.NewBufferString(testCase.content), testCase.path)
			if testCase.shouldPass {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestWriteFileOptions(t *testing.T) {
	for opName, opCases := range map[string]func(t *testing.T){
		"Validate": func(t *testing.T) {
			for testName, testCase := range map[string]func(t *testing.T){
				"FailsForZeroValue": func(t *testing.T) {
					opts := WriteFile{}
					assert.Error(t, opts.Validate())
				},
				"OnlyDefaultsPermForZeroValue": func(t *testing.T) {
					opts := WriteFile{Path: "/foo", Perm: 0777}
					assert.NoError(t, opts.Validate())
					assert.EqualValues(t, 0777, opts.Perm)
				},
				"PassesAndDefaults": func(t *testing.T) {
					opts := WriteFile{Path: "/foo"}
					assert.NoError(t, opts.Validate())
					assert.NotEqual(t, os.FileMode(0000), opts.Perm)
				},
				"PassesWithContent": func(t *testing.T) {
					opts := WriteFile{
						Path:    "/foo",
						Content: []byte("foo"),
					}
					assert.NoError(t, opts.Validate())
				},
				"PassesWithReader": func(t *testing.T) {
					opts := WriteFile{
						Path:   "/foo",
						Reader: bytes.NewBufferString("foo"),
					}
					assert.NoError(t, opts.Validate())
				},
				"FailsWithMultipleContentSources": func(t *testing.T) {
					opts := WriteFile{
						Path:    "/foo",
						Content: []byte("foo"),
						Reader:  bytes.NewBufferString("bar"),
					}
					assert.Error(t, opts.Validate())
				},
			} {
				t.Run(testName, func(t *testing.T) {
					testCase(t)
				})
			}
		},
		"ContentReader": func(t *testing.T) {
			for testName, testCase := range map[string]func(t *testing.T, opts WriteFile){
				"RequiresOneContentSource": func(t *testing.T, opts WriteFile) {
					opts.Content = []byte("foo")
					opts.Reader = bytes.NewBufferString("bar")
					_, err := opts.ContentReader()
					assert.Error(t, err)
				},
				"PreservesReaderIfSet": func(t *testing.T, opts WriteFile) {
					expected := []byte("foo")
					opts.Reader = bytes.NewBuffer(expected)
					reader, err := opts.ContentReader()
					require.NoError(t, err)
					assert.Equal(t, opts.Reader, reader)

					content, err := ioutil.ReadAll(reader)
					require.NoError(t, err)
					assert.Equal(t, expected, content)
				},
				"SetsReaderIfContentSet": func(t *testing.T, opts WriteFile) {
					expected := []byte("foo")
					opts.Content = expected
					reader, err := opts.ContentReader()
					require.NoError(t, err)
					assert.Equal(t, reader, opts.Reader)
					assert.Empty(t, opts.Content)

					content, err := ioutil.ReadAll(reader)
					require.NoError(t, err)
					assert.Equal(t, expected, content)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					opts := WriteFile{Path: "/path"}
					testCase(t, opts)
				})
			}
		},
		"WriteBufferedContent": func(t *testing.T) {
			for testName, testCase := range map[string]func(t *testing.T, opts WriteFile){
				"DoesNotErrorWithoutContentSource": func(t *testing.T, opts WriteFile) {
					didWrite := false
					assert.NoError(t, opts.WriteBufferedContent(func(WriteFile) error {
						didWrite = true
						return nil
					}))
					assert.True(t, didWrite)
				},
				"FailsForMultipleContentSources": func(t *testing.T, opts WriteFile) {
					opts.Content = []byte("foo")
					opts.Reader = bytes.NewBufferString("bar")
					assert.Error(t, opts.WriteBufferedContent(func(WriteFile) error { return nil }))
				},
				"ReadsFromContent": func(t *testing.T, opts WriteFile) {
					expected := []byte("foo")
					opts.Content = expected
					content := []byte{}
					require.NoError(t, opts.WriteBufferedContent(func(opts WriteFile) error {
						content = append(content, opts.Content...)
						return nil
					}))
					assert.Equal(t, expected, content)
				},
				"ReadsFromReader": func(t *testing.T, opts WriteFile) {
					expected := []byte("foo")
					opts.Reader = bytes.NewBuffer(expected)
					content := []byte{}
					require.NoError(t, opts.WriteBufferedContent(func(opts WriteFile) error {
						content = append(content, opts.Content...)
						return nil
					}))
					assert.Equal(t, expected, content)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					opts := WriteFile{Path: "/path"}
					testCase(t, opts)
				})
			}
		},
		"DoWrite": func(t *testing.T) {
			content := []byte("foo")
			for testName, testCase := range map[string]func(t *testing.T, opts WriteFile){
				"AllowsEmptyWriteToCreateFile": func(t *testing.T, opts WriteFile) {
					require.NoError(t, opts.DoWrite())

					stat, err := os.Stat(opts.Path)
					require.NoError(t, err)
					assert.Zero(t, stat.Size())
				},
				"WritesWithReader": func(t *testing.T, opts WriteFile) {
					opts.Reader = bytes.NewBuffer(content)

					require.NoError(t, opts.DoWrite())

					fileContent, err := ioutil.ReadFile(opts.Path)
					require.NoError(t, err)
					assert.Equal(t, content, fileContent)
				},
				"WritesWithContent": func(t *testing.T, opts WriteFile) {
					opts.Content = content

					require.NoError(t, opts.DoWrite())

					fileContent, err := ioutil.ReadFile(opts.Path)
					require.NoError(t, err)
					assert.Equal(t, content, fileContent)
				},
				"AppendsToFile": func(t *testing.T, opts WriteFile) {
					f, err := os.OpenFile(opts.Path, os.O_WRONLY|os.O_CREATE, 0666)
					initialContent := []byte("bar")
					require.NoError(t, err)
					_, err = f.Write(initialContent)
					require.NoError(t, err)
					require.NoError(t, f.Close())

					opts.Append = true
					opts.Content = []byte(content)

					require.NoError(t, opts.DoWrite())

					fileContent, err := ioutil.ReadFile(opts.Path)
					require.NoError(t, err)
					assert.Equal(t, initialContent, fileContent[:len(initialContent)])
					assert.Equal(t, content, fileContent[len(fileContent)-len(content):])
				},
				"TruncatesExistingFile": func(t *testing.T, opts WriteFile) {
					f, err := os.OpenFile(opts.Path, os.O_WRONLY|os.O_CREATE, 0666)
					initialContent := []byte("bar")
					require.NoError(t, err)
					_, err = f.Write(initialContent)
					require.NoError(t, err)
					require.NoError(t, f.Close())

					opts.Content = []byte(content)

					require.NoError(t, opts.DoWrite())

					fileContent, err := ioutil.ReadFile(opts.Path)
					require.NoError(t, err)
					assert.Equal(t, content, fileContent)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					cwd, err := os.Getwd()
					require.NoError(t, err)
					opts := WriteFile{Path: filepath.Join(filepath.Dir(cwd), "build", filepath.Base(t.Name()))}
					defer func() {
						assert.NoError(t, os.RemoveAll(opts.Path))
					}()
					testCase(t, opts)
				})
			}
		},
		"SetPerm": func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("permission tests are not relevant to Windows")
			}
			for testName, testCase := range map[string]func(t *testing.T, opts WriteFile){
				"SetsPermissions": func(t *testing.T, opts WriteFile) {
					f, err := os.OpenFile(opts.Path, os.O_RDWR|os.O_CREATE, 0666)
					require.NoError(t, err)
					require.NoError(t, f.Close())

					opts.Perm = 0400
					require.NoError(t, opts.SetPerm())

					stat, err := os.Stat(opts.Path)
					require.NoError(t, err)
					assert.Equal(t, opts.Perm, stat.Mode())
				},
				"FailsWithoutFile": func(t *testing.T, opts WriteFile) {
					opts.Perm = 0400
					assert.Error(t, opts.SetPerm())
				},
			} {
				t.Run(testName, func(t *testing.T) {
					cwd, err := os.Getwd()
					require.NoError(t, err)
					opts := WriteFile{Path: filepath.Join(filepath.Dir(cwd), "build", filepath.Base(t.Name()))}
					defer func() {
						assert.NoError(t, os.RemoveAll(opts.Path))
					}()
					testCase(t, opts)
				})
			}
		},
	} {
		t.Run(opName, func(t *testing.T) {
			opCases(t)
		})
	}
}
