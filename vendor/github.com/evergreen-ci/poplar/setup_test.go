package poplar

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	yaml "gopkg.in/yaml.v2"
)

func TestReportSetup(t *testing.T) {
	tmpDir, err := ioutil.TempDir(".", "setup_test")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tmpDir))
	}()

	expectedReport := &Report{
		Project:   "project",
		Version:   "version",
		Order:     101,
		Variant:   "variant",
		TaskName:  "task_name",
		Execution: 5,
		Mainline:  true,
		BucketConf: BucketConfiguration{
			APIKey:    "key",
			APISecret: "secret",
			APIToken:  "token",
			Name:      "bucket",
			Prefix:    "pre",
			Region:    "east",
		},
		Tests: []Test{},
	}

	t.Run("InvalidReportType", func(t *testing.T) {
		report, err := ReportSetup("invalid", "")
		assert.Error(t, err)
		assert.Nil(t, report)
	})
	t.Run("InvalidFileName", func(t *testing.T) {
		report, err := ReportSetup(ReportTypeJSON, "DNE")
		assert.Error(t, err)
		assert.Nil(t, report)
	})
	t.Run("Marshal", func(t *testing.T) {
		for _, test := range []struct {
			name       string
			marshal    func(interface{}) ([]byte, error)
			reportType ReportType
		}{
			{
				name:       "JSON",
				marshal:    json.Marshal,
				reportType: ReportTypeJSON,
			},
			{
				name:       "BSON",
				marshal:    bson.Marshal,
				reportType: ReportTypeBSON,
			},
			{
				name:       "YAML",
				marshal:    yaml.Marshal,
				reportType: ReportTypeYAML,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				data, err := test.marshal(expectedReport)
				require.NoError(t, err)
				filename := filepath.Join(tmpDir, fmt.Sprintf("test.%s", test.name))
				file, err := os.Create(filename)
				require.NoError(t, err)
				defer func() {
					assert.NoError(t, file.Close())
				}()
				_, err = file.Write(data)
				require.NoError(t, err)

				report, err := ReportSetup(test.reportType, filename)
				assert.NoError(t, err)
				assert.Equal(t, expectedReport, report)
			})
		}
	})
	t.Run("Env", func(t *testing.T) {
		require.NoError(t, os.Setenv(ProjectEnv, expectedReport.Project))
		require.NoError(t, os.Setenv(VersionEnv, expectedReport.Version))
		require.NoError(t, os.Setenv(OrderEnv, fmt.Sprintf("%d", expectedReport.Order)))
		require.NoError(t, os.Setenv(VariantEnv, expectedReport.Variant))
		require.NoError(t, os.Setenv(TaskNameEnv, expectedReport.TaskName))
		require.NoError(t, os.Setenv(ExecutionEnv, fmt.Sprintf("%d", expectedReport.Execution)))
		require.NoError(t, os.Setenv(MainlineEnv, fmt.Sprintf("%v", expectedReport.Mainline)))
		require.NoError(t, os.Setenv(APIKeyEnv, expectedReport.BucketConf.APIKey))
		require.NoError(t, os.Setenv(APISecretEnv, expectedReport.BucketConf.APISecret))
		require.NoError(t, os.Setenv(APITokenEnv, expectedReport.BucketConf.APIToken))
		require.NoError(t, os.Setenv(BucketNameEnv, expectedReport.BucketConf.Name))
		require.NoError(t, os.Setenv(BucketPrefixEnv, expectedReport.BucketConf.Prefix))
		require.NoError(t, os.Setenv(BucketRegionEnv, expectedReport.BucketConf.Region))

		t.Run("ValidVars", func(t *testing.T) {
			report, err := ReportSetup(ReportTypeEnv, "")
			assert.NoError(t, err)
			assert.Equal(t, expectedReport, report)
		})
		t.Run("PatchBuild", func(t *testing.T) {
			require.NoError(t, os.Setenv(MainlineEnv, fmt.Sprintf("%v", false)))
			require.NoError(t, os.Setenv(OrderEnv, "NOT_AN_INT"))
			report, err := ReportSetup(ReportTypeEnv, "")
			assert.NoError(t, err)
			assert.False(t, report.Mainline)
			assert.Zero(t, report.Order)
			require.NoError(t, os.Setenv(MainlineEnv, fmt.Sprintf("%v", expectedReport.Mainline)))
			require.NoError(t, os.Setenv(OrderEnv, fmt.Sprintf("%d", expectedReport.Order)))
		})
		t.Run("InvalidOrder", func(t *testing.T) {
			require.NoError(t, os.Setenv(OrderEnv, "NOT_AN_INT"))
			report, err := ReportSetup(ReportTypeEnv, "")
			assert.Error(t, err)
			assert.Nil(t, report)
			require.NoError(t, os.Setenv(OrderEnv, fmt.Sprintf("%d", expectedReport.Order)))
		})
		t.Run("InvalidExecution", func(t *testing.T) {
			require.NoError(t, os.Setenv(ExecutionEnv, "NOT_AN_INT"))
			report, err := ReportSetup(ReportTypeEnv, "")
			assert.Error(t, err)
			assert.Nil(t, report)
			require.NoError(t, os.Setenv(ExecutionEnv, fmt.Sprintf("%d", expectedReport.Execution)))
		})
		t.Run("InvalidMainline", func(t *testing.T) {
			require.NoError(t, os.Setenv(MainlineEnv, "NOT_A_BOOL"))
			report, err := ReportSetup(ReportTypeEnv, "")
			assert.Error(t, err)
			assert.Nil(t, report)
			require.NoError(t, os.Setenv(MainlineEnv, fmt.Sprintf("%v", expectedReport.Mainline)))
		})
	})
}
