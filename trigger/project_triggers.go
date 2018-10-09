package trigger

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
)

func TriggerDownstreamVersion(source *version.Version, downstreamProject, file, command string) (*version.Version, error) {
	if command != "" {
		return nil, errors.New("command-based triggers are not yet implemented")
	}
	if file != "" && command != "" {
		return nil, errors.New("cannot specify both a file and command")
	}
	if source == nil {
		return nil, errors.Errorf("unable to find source version in project %s", downstreamProject)
	}
	ref, err := model.FindOneProjectRef(downstreamProject)
	if err != nil {
		return nil, errors.Wrap(err, "error finding project ref")
	}
	if ref == nil {
		return nil, errors.Errorf("unable to find project ref %s", downstreamProject)
	}

	// propogate version metadata to the downstream version
	rev, err := revisionFromVersion(*source)
	if err != nil {
		return nil, err
	}

	// get the downstream config
	config, err := makeDownstreamConfigFromFile(*ref, file)
	if err != nil {
		return nil, err
	}

	return repotracker.CreateVersionFromConfig(ref, config, rev, false, nil)
}

func revisionFromVersion(source version.Version) (*model.Revision, error) {
	rev := model.Revision{
		Author:          source.Author,
		AuthorEmail:     source.AuthorEmail,
		CreateTime:      source.CreateTime,
		RevisionMessage: source.Message,
		Revision:        source.Revision,
	}
	author, err := user.FindOneById(source.AuthorID)
	if err != nil {
		return nil, errors.Wrap(err, "error finding version author")
	}
	if author != nil {
		rev.AuthorGithubUID = author.Settings.GithubUser.UID
	}

	return &rev, nil
}

func makeDownstreamConfigFromFile(ref model.ProjectRef, file string) (*model.Project, error) {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error getting evergreen settings")
	}
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	configFile, err := thirdparty.GetGithubFile(ctx, token, ref.Owner, ref.Repo, file, "")
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching project file for '%s'", ref.Identifier)
	}
	fileContents, err := base64.StdEncoding.DecodeString(*configFile.Content)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to decode config file for '%s'", ref.Identifier)
	}

	config := model.Project{}
	err = model.LoadProjectInto(fileContents, ref.Identifier, &config)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing config file for '%s'", ref.Identifier)
	}
	return &config, nil
}
