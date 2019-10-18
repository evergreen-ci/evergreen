package timber

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	yaml "gopkg.in/yaml.v2"
)

func TestLoadLoggerOptions(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "load-logger-test")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tmpDir))
	}()
	expectedOptions := &LoggerOptions{
		Project:     "project",
		Version:     "version",
		Variant:     "variant",
		TaskName:    "task_name",
		TaskID:      "task_id",
		Execution:   2,
		TestName:    "test_name",
		Trial:       3,
		ProcessName: "proc",
		Format:      LogFormatJSON,
		Tags:        []string{"tag1", "tag2", "tag3"},
		Arguments:   map[string]string{"arg1": "key1", "arg2": "key2"},
		Mainline:    true,

		Storage: LogStorageS3,

		MaxBufferSize: 1024,
		FlushInterval: time.Minute,

		DisableNewLineCheck: true,

		RPCAddress: "mongodb.cedar.com",
		Insecure:   false,
		CAFile:     "ca.crt",
		CertFile:   "user.crt",
		KeyFile:    "user.key",
	}

	for _, test := range []struct {
		name       string
		fn         string
		marshaller func(interface{}) ([]byte, error)
		createFile bool
		hasErr     bool
	}{
		{
			name:   "FileDoesNotExist",
			fn:     "DNE",
			hasErr: true,
		},
		{
			name:   "FileIsDir",
			fn:     tmpDir,
			hasErr: true,
		},
		{
			name:       "NoUnmarshaler",
			fn:         filepath.Join(tmpDir, "options.csv"),
			createFile: true,
			hasErr:     true,
		},
		{
			name:       "MarshalsBSONCorrectly",
			fn:         filepath.Join(tmpDir, "options.bson"),
			marshaller: bson.Marshal,
		},

		{
			name:       "MarshalsJSONCorrectly",
			fn:         filepath.Join(tmpDir, "options.json"),
			marshaller: json.Marshal,
		},
		{
			name:       "MarshalsYAMLCorrectlyYML",
			fn:         filepath.Join(tmpDir, "options.yml"),
			marshaller: yaml.Marshal,
		},
		{
			name:       "MarshalsYAMLCorrectlyYAML",
			fn:         filepath.Join(tmpDir, "options.yaml"),
			marshaller: yaml.Marshal,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.hasErr {
				if test.createFile {
					f, err := os.Create(test.fn)
					require.NoError(t, err)
					require.NoError(t, f.Close())
				}
				options, err := LoadLoggerOptions(test.fn)
				assert.Nil(t, options)
				assert.Error(t, err)
			} else {
				data, err := test.marshaller(expectedOptions)
				require.NoError(t, err)
				f, err := os.Create(test.fn)
				require.NoError(t, err)
				_, err = f.Write(data)
				assert.NoError(t, f.Close())
				require.NoError(t, err)

				options, err := LoadLoggerOptions(test.fn)
				assert.Equal(t, expectedOptions, options)
				assert.NoError(t, err)
			}
		})
	}
}
