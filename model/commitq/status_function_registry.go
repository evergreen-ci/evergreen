package commitq

import (
	"github.com/evergreen-ci/evergreen/thirdparty"
)

type StatusFunction interface{}

var statusMethods map[string]StatusFunction

func init() {
	statusMethods = map[string]StatusFunction{
		"github": thirdparty.GithubPostStatus,
	}
}

func GetStatusAction(actionString string) StatusFunction {
	action, ok := statusMethods[actionString]
	if !ok {
		return nil
	}
	return action
}
