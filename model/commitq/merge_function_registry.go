package commitq

import (
	"errors"

	"github.com/evergreen-ci/evergreen/thirdparty"
)

type MergeArgs struct {
	Title   string
	Message string
	SHA     string
}

type MergeFunction func(MergeArgs) error

var mergeMethods map[string]MergeFunction

func githubMergeFunction(action string) func(MergeArgs) error {
	return func(args MergeArgs) error {
		mergeArgs := thirdparty.GithubMergeParams{
			Title:       args.Title,
			Message:     args.Message,
			SHA:         args.SHA,
			MergeMethod: action,
		}
		return thirdparty.GithubPRMerge(mergeArgs)
	}
}

func init() {
	mergeMethods = map[string]MergeFunction{
		"merge":  githubMergeFunction("merge"),
		"squash": githubMergeFunction("squash"),
		"rebase": githubMergeFunction("rebase"),
	}
}

func GetMergeAction(actionString string) (MergeFunction, error) {
	action, ok := mergeMethods[actionString]
	if !ok {
		return nil, errors.New("no matching merge action")
	}
	return action, nil
}
