package poplar

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReport(t *testing.T) {
	data, err := ioutil.ReadFile("testdata/report.json")
	require.NoError(t, err)
	expectedReport := &Report{}
	require.NoError(t, json.Unmarshal(data, expectedReport))

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
