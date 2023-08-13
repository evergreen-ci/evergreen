package taskbuild

import "fmt"

const Root = "build"

type TaskOptions struct {
	ProjectID        string
	TaskID           string
	Execution        int
	TaskBuildVersion int
}

func (o TaskOptions) Prefix() string {
	switch {
	case o.TaskBuildVersion >= 0:
		return fmt.Sprintf("%s/%s/%d/%s", o.ProjectID, o.TaskID, o.Execution, Root)
	default:
		return ""
	}
}
