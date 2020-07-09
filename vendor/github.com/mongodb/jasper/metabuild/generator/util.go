package generator

import (
	"strings"

	"github.com/evergreen-ci/shrub"
	"github.com/mongodb/jasper/metabuild/model"
	"github.com/pkg/errors"
)

const (
	// minTasksForTaskGroup is the minimum number of tasks that have to be in a
	// task group in order for it to be worthwhile to create a task group. Since
	// max hosts must can be at most half the number of tasks and we don't want
	// to use single-host task groups, we must have at least four tasks in the
	// group to make a multi-host task group.
	minTasksForTaskGroup = 4

	// taskGroupSuffix is used to name task groups.
	taskGroupSuffix = "_group"
)

// getTaskName returns an auto-generated task name.
func getTaskName(parts ...string) string {
	return strings.Join(parts, "-")
}

// getTaskGroupName returns an auto-generated task group name.
func getTaskGroupName(name string) string {
	return name + taskGroupSuffix
}

// fileReportCmds converts the given files to report to the evergreen command
// that will report the file.
func fileReportCmds(frs ...model.FileReport) ([]shrub.Command, error) {
	var cmds []shrub.Command
	for _, fr := range frs {
		cmd, err := fileReportCmd(fr)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cmds = append(cmds, cmd)
	}
	return cmds, nil
}

// fileReportCmd converts a single file report to the evergreen command that
// will report the file.
func fileReportCmd(fr model.FileReport) (shrub.Command, error) {
	switch fr.Format {
	case model.Artifact:
		return shrub.CmdAttachArtifacts{
			Files: fr.Files,
		}, nil
	case model.EvergreenJSON:
		if len(fr.Files) != 1 {
			return nil, errors.New("evergreen JSON results format requires exactly one test results file to be provided")
		}
		return shrub.CmdResultsJSON{
			File: fr.Files[0],
		}, nil
	case model.GoTest:
		return shrub.CmdResultsGoTest{
			LegacyFormat: true,
			Files:        fr.Files,
		}, nil
	case model.XUnit:
		return shrub.CmdResultsXunit{
			Files: fr.Files,
		}, nil
	default:
		return nil, errors.Errorf("unrecognized file report format '%s'", fr.Format)
	}
}
