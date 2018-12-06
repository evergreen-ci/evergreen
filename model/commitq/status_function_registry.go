package commitq

import (
	"errors"

	"github.com/evergreen-ci/evergreen/thirdparty"
)

type StatusFunction func(thirdparty.GithubStatusParams) error

var statusMethods map[string]StatusFunction

func init() {
	statusMethods = map[string]StatusFunction{
		"github": thirdparty.GithubPostStatus,
	}
}

func GetStatusAction(actionString string) (StatusFunction, error) {
	action, ok := statusMethods[actionString]
	if !ok {
		return nil, errors.New("no matching status action")
	}
	return action, nil
}
