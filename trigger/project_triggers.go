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

// TriggerDownstreamVersion assumes that you definitely want to create a downstream version
// and will go through the process of version creation given a triggering version
func TriggerDownstreamVersion(args ProcessorArgs) (*version.Version, error) {
	if args.Command != "" {
		return nil, errors.New("command-based triggers are not yet implemented")
	}
	if args.File != "" && args.Command != "" {
		return nil, errors.New("cannot specify both a file and command")
	}
	if args.SourceVersion == nil {
		return nil, errors.Errorf("unable to find source version in project %s", args.DownstreamProject.Identifier)
	}

	// propagate version metadata to the downstream version
	metadata, err := metadataFromVersion(*args.SourceVersion, args.DownstreamProject)
	if err != nil {
		return nil, err
	}
	metadata.TriggerID = args.TriggerID
	metadata.TriggerType = args.TriggerType
	metadata.EventID = args.EventID
	metadata.DefinitionID = args.DefinitionID

	// get the downstream config
	config, err := makeDownstreamConfigFromFile(args.DownstreamProject, args.File)
	if err != nil {
		return nil, err
	}

	// create version
	v, err := repotracker.CreateVersionFromConfig(&args.DownstreamProject, config, metadata, false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error creating version")
	}
	err = model.UpdateLastRevision(v.Identifier, v.Revision)
	if err != nil {
		return nil, errors.Wrap(err, "error updating last revision")
	}
	err = repotracker.AddBuildBreakSubscriptions(v, &args.DownstreamProject)
	if err != nil {
		return nil, errors.Wrap(err, "error adding build break subscriptions")
	}

	return v, model.DoProjectActivation(args.DownstreamProject.Identifier)
}

func metadataFromVersion(source version.Version, ref model.ProjectRef) (repotracker.VersionMetadata, error) {
	metadata := repotracker.VersionMetadata{
		SourceVersion: &source,
	}
	metadata.Revision = &model.Revision{
		Author:          source.Author,
		AuthorEmail:     source.AuthorEmail,
		CreateTime:      source.CreateTime,
		RevisionMessage: source.Message,
	}
	repo, err := model.FindRepository(ref.Identifier)
	if err != nil {
		return metadata, errors.Wrap(err, "error finding most recent revision")
	}
	metadata.Revision.Revision = repo.LastRevision
	author, err := user.FindOneById(source.AuthorID)
	if err != nil {
		return metadata, errors.Wrap(err, "error finding version author")
	}
	if author != nil {
		metadata.Revision.AuthorGithubUID = author.Settings.GithubUser.UID
	}

	return metadata, nil
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
