package commitq

import (
	"github.com/evergreen-ci/evergreen/thirdparty"
)

type MergeFunction interface{}

var mergeMethods map[string]MergeFunction

func init() {
	mergeMethods = map[string]MergeFunction{
		"merge": func(title string, message string, sha string) error {
			return thirdparty.GithubPRMerge(title, message, sha, "merge")
		},
		"squash": func(title string, message string, sha string) error {
			return thirdparty.GithubPRMerge(title, message, sha, "squash")
		},
		"rebase": func(title string, message string, sha string) error {
			return thirdparty.GithubPRMerge(title, message, sha, "rebase")
		},
	}
}

func GetMergeAction(actionString string) MergeFunction {
	action, ok := mergeMethods[actionString]
	if !ok {
		return nil
	}
	return action
}
