package trigger

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/pkg/errors"
)

// TriggerDownstreamVersion assumes that you definitely want to create a downstream version
// and will go through the process of version creation given a triggering version
func TriggerDownstreamVersion(args ProcessorArgs) (*model.Version, error) {
	if args.SourceVersion == nil {
		return nil, errors.Errorf("unable to find source version in downstream project '%s'", args.DownstreamProject.Id)
	}

	// propagate version metadata to the downstream version
	metadata, err := metadataFromVersion(*args.SourceVersion, args.DownstreamProject)
	if err != nil {
		return nil, err
	}
	metadata.TriggerID = args.TriggerID
	metadata.TriggerType = args.TriggerType
	metadata.EventID = args.EventID
	metadata.TriggerDefinitionID = args.DefinitionID
	metadata.Alias = args.Alias

	// get the downstream config
	projectInfo := model.ProjectInfo{}
	if args.ConfigFile != "" {
		projectInfo, err = makeDownstreamProjectFromFile(args.DownstreamProject, args.ConfigFile)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else {
		return nil, errors.New("must specify a file to define downstream project config")
	}

	// create version
	projectInfo.Ref = &args.DownstreamProject
	v, err := repotracker.CreateVersionFromConfig(context.Background(), &projectInfo, metadata, false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating version")
	}
	err = args.SourceVersion.AddSatisfiedTrigger(args.DefinitionID)
	if err != nil {
		return nil, err
	}
	settings, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "getting Evergreen settings")
	}
	upstreamProject, err := model.FindMergedProjectRef(args.SourceVersion.Identifier, args.SourceVersion.Id, true)
	if err != nil {
		return nil, errors.Wrapf(err, "finding project ref '%s' for source version '%s'", args.SourceVersion.Identifier, args.SourceVersion.Id)
	}
	if upstreamProject == nil {
		return nil, errors.Errorf("upstream project '%s' not found", args.SourceVersion.Identifier)
	}
	for _, module := range projectInfo.Project.Modules {
		owner, repo := module.GetRepoOwnerAndName()
		if owner == upstreamProject.Owner && repo == upstreamProject.Repo && module.Branch == upstreamProject.Branch {
			_, err = model.CreateManifest(v, projectInfo.Project, upstreamProject, settings)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			break
		}
	}
	err = model.UpdateLastRevision(v.Identifier, v.Revision)
	if err != nil {
		return nil, errors.Wrap(err, "updating last revision")
	}
	err = repotracker.AddBuildBreakSubscriptions(v, &args.DownstreamProject)
	if err != nil {
		return nil, errors.Wrap(err, "adding build break subscriptions")
	}
	return v, nil
}

func metadataFromVersion(source model.Version, ref model.ProjectRef) (model.VersionMetadata, error) {
	metadata := model.VersionMetadata{
		SourceVersion: &source,
	}
	metadata.Revision = model.Revision{
		Author:          source.Author,
		AuthorEmail:     source.AuthorEmail,
		CreateTime:      source.CreateTime,
		RevisionMessage: source.Message,
	}
	repo, err := model.FindRepository(ref.Id)
	if err != nil {
		return metadata, errors.Wrap(err, "finding most recent revision")
	}
	metadata.Revision.Revision = repo.LastRevision
	author, err := user.FindOneById(source.AuthorID)
	if err != nil {
		return metadata, errors.Wrapf(err, "finding version author '%s'", source.AuthorID)
	}
	if author != nil {
		metadata.Revision.AuthorGithubUID = author.Settings.GithubUser.UID
	}

	return metadata, nil
}

func makeDownstreamProjectFromFile(ref model.ProjectRef, file string) (model.ProjectInfo, error) {
	opts := model.GetProjectOpts{
		Ref:        &ref,
		RemotePath: file,
		Revision:   ref.Branch,
	}
	settings, err := evergreen.GetConfig()
	if err != nil {
		return model.ProjectInfo{}, errors.Wrap(err, "getting Evergreen settings")
	}
	opts.Token, err = settings.GetGithubOauthToken()
	if err != nil {
		return model.ProjectInfo{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	return model.GetProjectFromFile(ctx, opts)
}
