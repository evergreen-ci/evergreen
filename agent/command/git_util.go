package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type cloneCMD struct {
	// Required fields.
	owner string
	repo  string
	dir   string

	// Optional fields.
	branch            string
	token             string
	recurseSubmodules bool
	useVerbose        bool
	cloneDepth        int
}

func (opts cloneCMD) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.owner == "", "missing required owner")
	catcher.NewWhen(opts.repo == "", "missing required repo")
	catcher.NewWhen(opts.dir == "", "missing required directory")

	catcher.NewWhen(opts.cloneDepth < 0, "clone depth cannot be negative")
	return catcher.Resolve()
}

func (opts cloneCMD) getCloneCommands() ([]string, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid clone command options")
	}

	gitURL := thirdparty.FormGitURLForApp(opts.owner, opts.repo, opts.token)

	clone := fmt.Sprintf("git clone %s '%s'", gitURL, opts.dir)

	if opts.recurseSubmodules {
		clone = fmt.Sprintf("%s --recurse-submodules", clone)
	}
	if opts.useVerbose {
		clone = fmt.Sprintf("GIT_TRACE=1 %s", clone)
	}
	if opts.cloneDepth > 0 {
		clone = fmt.Sprintf("%s --depth %d", clone, opts.cloneDepth)
	}
	if opts.branch != "" {
		clone = fmt.Sprintf("%s --branch '%s'", clone, opts.branch)
	}

	return []string{
		"set +o xtrace",
		fmt.Sprintf(`echo %s`, strconv.Quote(clone)),
		clone,
		"set -o xtrace",
		fmt.Sprintf("cd %s", opts.dir),
	}, nil
}

// getCloneToken returns the project's clone method and token. If
// set, the project token takes precedence over GitHub App token which takes precedence over over global settings.
func getCloneToken(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig, providedToken string) (string, error) {
	if providedToken != "" {
		token, err := parseToken(providedToken)
		return token, err
	}

	owner := conf.ProjectRef.Owner
	repo := conf.ProjectRef.Repo
	appToken, err := comm.CreateInstallationTokenForClone(ctx, conf.TaskData(), owner, repo)
	if err != nil {
		return "", errors.Wrap(err, "creating app token")
	}
	if appToken != "" {
		// Redact the token from the logs.
		conf.NewExpansions.Redact(generatedTokenKey, appToken)
	}

	return appToken, nil
}

// parseToken parses the OAuth token, if it is in the format "token <token>";
// otherwise, it returns the token unchanged.
func parseToken(token string) (string, error) {
	if !strings.HasPrefix(token, "token") {
		return token, nil
	}
	splitToken := strings.Split(token, " ")
	if len(splitToken) != 2 {
		return "", errors.New("token format is invalid")
	}
	return splitToken[1], nil
}
