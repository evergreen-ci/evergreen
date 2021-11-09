package poplar

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReport(t *testing.T) {
	data, err := ioutil.ReadFile("testdata/report.json")
	require.NoError(t, err)
	expectedReport := &Report{}
	require.NoError(t, json.Unmarshal(data, expectedReport))

	require.NoError(t, os.Setenv(APIKeyEnv, "key"))
	require.NoError(t, os.Setenv(APISecretEnv, "secret"))
	require.NoError(t, os.Setenv(APITokenEnv, "token"))
	require.Empty(t, expectedReport.BucketConf.APIKey)
	require.Empty(t, expectedReport.BucketConf.APISecret)
	require.Empty(t, expectedReport.BucketConf.APIToken)
	expectedReport.BucketConf.APIKey = "key"
	expectedReport.BucketConf.APISecret = "secret"
	expectedReport.BucketConf.APIToken = "token"

	for _, test := range []struct {
		name   string
		fn     string
		hasErr bool
	}{
		{
			name:   "FileDoesNotExist",
			fn:     "DNE",
			hasErr: true,
		},
		{
			name:   "FileIsDir",
			fn:     "testdata",
			hasErr: true,
		},
		{
			name:   "NoUnmarshaler",
			fn:     "testdata/csv_example.csv",
			hasErr: true,
		},
		{
			name: "MarshalsBSONCorrectly",
			fn:   "testdata/report.bson",
		},
		{
			name: "MarshalsJSONCorrectly",
			fn:   "testdata/report.json",
		},
		{
			name: "MarshalsYAMLCorrectly",
			fn:   "testdata/report.yaml",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			report, err := LoadReport(test.fn)
			if test.hasErr {
				assert.Nil(t, report)
				assert.Error(t, err)
			} else {
				assert.Equal(t, expectedReport, report)
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadTests(t *testing.T) {
	data, err := ioutil.ReadFile("testdata/tests.json")
	require.NoError(t, err)
	expectedTests := []Test{}
	require.NoError(t, json.Unmarshal(data, &expectedTests))
	expectedReport := &Report{Tests: expectedTests}

	for _, test := range []struct {
		name   string
		fn     string
		hasErr bool
	}{
		{
			name:   "FileDoesNotExist",
			fn:     "DNE",
			hasErr: true,
		},
		{
			name:   "FileIsDir",
			fn:     "testdata",
			hasErr: true,
		},
		{
			name:   "NoUnmarshaler",
			fn:     "testdata/csv_example.csv",
			hasErr: true,
		},
		{
			name:   "MarshalsBSONFails",
			fn:     "testdata/report.bson",
			hasErr: true,
		},
		{
			name: "MarshalsJSONCorrectly",
			fn:   "testdata/tests.json",
		},
		{
			name: "MarshalsYAMLCorrectly",
			fn:   "testdata/tests.yaml",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			report, err := LoadTests(test.fn)
			if test.hasErr {
				assert.Nil(t, report)
				assert.Error(t, err)
			} else {
				assert.Equal(t, expectedReport, report)
				assert.NoError(t, err)
			}
		})
	}
}
