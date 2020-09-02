package generator

import (
	"testing"

	"github.com/evergreen-ci/shrub"
	"github.com/mongodb/jasper/metabuild/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileReportCmds(t *testing.T) {
	t.Run("FileReportCmd", func(t *testing.T) {
		for testName, testParams := range map[string]struct {
			format   model.ReportFormat
			testCase func(*testing.T, model.FileReport, *shrub.CommandDefinition)
		}{
			"CreatesCommandFromArtifact": {
				format: model.Artifact,
				testCase: func(t *testing.T, fr model.FileReport, cmd *shrub.CommandDefinition) {
					assert.Equal(t, shrub.CmdAttachArtifacts{}.Name(), cmd.CommandName)
					i, ok := cmd.Params["files"]
					require.True(t, ok)
					files, ok := i.([]interface{})
					require.True(t, ok)
					require.Len(t, files, len(fr.Files))
					assert.Equal(t, fr.Files[0], files[0])
				},
			},
			"CreatesCommandFromEvergreenJSON": {
				format: model.EvergreenJSON,
				testCase: func(t *testing.T, fr model.FileReport, cmd *shrub.CommandDefinition) {
					assert.Equal(t, shrub.CmdResultsJSON{}.Name(), cmd.CommandName)
					file, ok := cmd.Params["file_location"]
					require.True(t, ok)
					assert.Equal(t, fr.Files[0], file)
				},
			},
			"CreatesCommandFromGoTest": {
				format: model.GoTest,
				testCase: func(t *testing.T, fr model.FileReport, cmd *shrub.CommandDefinition) {
					assert.Equal(t, shrub.CmdResultsGoTest{LegacyFormat: true}.Name(), cmd.CommandName)
					i, ok := cmd.Params["files"]
					require.True(t, ok)
					files, ok := i.([]interface{})
					require.True(t, ok)
					require.Len(t, files, len(fr.Files))
					assert.Equal(t, fr.Files[0], files[0])
				},
			},
			"CreatesCommandFromXUnit": {
				format: model.XUnit,
				testCase: func(t *testing.T, fr model.FileReport, cmd *shrub.CommandDefinition) {
					assert.Equal(t, shrub.CmdResultsXunit{}.Name(), cmd.CommandName)
					i, ok := cmd.Params["files"]
					require.True(t, ok)
					files, ok := i.([]interface{})
					require.True(t, ok)
					require.Len(t, files, len(fr.Files))
					assert.Equal(t, fr.Files[0], files[0])
				},
			},
		} {
			t.Run(testName, func(t *testing.T) {
				fr := model.FileReport{
					Format: testParams.format,
					Files:  []string{"file"},
				}
				cmd, err := fileReportCmd(fr)
				require.NoError(t, err)
				testParams.testCase(t, fr, cmd)
			})
		}
		t.Run("FailsWithEvergreenJSONWithInvalidNumFiles", func(t *testing.T) {
			fr := model.FileReport{
				Format: model.EvergreenJSON,
				Files:  []string{"file1", "file2"},
			}
			cmd, err := fileReportCmd(fr)
			assert.Error(t, err)
			assert.Zero(t, cmd)
		})
		t.Run("FailsWithInvalidFormat", func(t *testing.T) {
			fr := model.FileReport{
				Format: "foo",
				Files:  []string{"file"},
			}
			cmd, err := fileReportCmd(fr)
			assert.Error(t, err)
			assert.Zero(t, cmd)
		})
	})

	t.Run("GeneratesCommandsForAllFileReports", func(t *testing.T) {
		frs := []model.FileReport{
			{
				Format: model.Artifact,
				Files:  []string{"file1"},
			},
			{
				Format: model.GoTest,
				Files:  []string{"file2"},
			},
			{
				Format: model.EvergreenJSON,
				Files:  []string{"file3"},
			},
			{
				Format: model.XUnit,
				Files:  []string{"file4", "file5"},
			},
		}
		cmds, err := fileReportCmds(frs...)
		require.NoError(t, err)
		assert.Len(t, cmds, len(frs))
	})
}
