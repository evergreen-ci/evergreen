package trigger

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
)

// TriggerDownstreamVersion assumes that you definitely want to create a downstream version
// and will go through the process of version creation given a triggering version.
// If the trigger is a push event, the triggering version will be nonexistent.
func TriggerDownstreamVersion(ctx context.Context, args ProcessorArgs) (*model.Version, error) {
	if args.SourceVersion == nil && args.TriggerType != model.ProjectTriggerLevelPush {
		return nil, errors.Errorf("unable to find source version in downstream project '%s'", args.DownstreamProject.Id)
	}

	// propagate version metadata to the downstream version
	metadata, err := metadataFromVersion(args)
	if err != nil {
		return nil, err
	}

	// get the downstream config
	projectInfo := model.ProjectInfo{}
	if args.ConfigFile != "" {
		projectInfo, err = makeDownstreamProjectFromFile(ctx, args.DownstreamProject, args.ConfigFile)
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
	if args.SourceVersion != nil {
		if err = args.SourceVersion.AddSatisfiedTrigger(args.DefinitionID); err != nil {
			return nil, err
		}
	}
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting Evergreen settings")
	}
	var versionID string
	// For build and task level triggers, there is a source version from which we can extract a project ID.
	// For push triggers, since there is no source version and the ProjectID field gets populated from
	// the trigger definition's project ID.
	projectID := args.ProjectID
	if projectID == "" {
		projectID = args.SourceVersion.Identifier
		versionID = args.SourceVersion.Id
	}
	upstreamProject, err := model.FindMergedProjectRef(projectID, versionID, true)
	if err != nil {
		return nil, errors.Wrapf(err, "finding project ref '%s' for source version '%s'", projectID, versionID)
	}
	if upstreamProject == nil {
		return nil, errors.Errorf("upstream project '%s' not found", projectID)
	}
	for _, module := range projectInfo.Project.Modules {
		owner, repo, err := thirdparty.ParseGitUrl(module.Repo)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing git url '%s'", module.Repo)
		}
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

func metadataFromVersion(args ProcessorArgs) (model.VersionMetadata, error) {
	metadata := model.VersionMetadata{
		SourceVersion:       args.SourceVersion,
		Activate:            !args.UnscheduleDownstreamVersions,
		TriggerID:           args.TriggerID,
		TriggerType:         args.TriggerType,
		EventID:             args.EventID,
		TriggerDefinitionID: args.DefinitionID,
		Alias:               args.Alias,
	}
	if args.SourceVersion != nil {
		metadata.Revision = model.Revision{
			Author:          args.SourceVersion.Author,
			AuthorEmail:     args.SourceVersion.AuthorEmail,
			CreateTime:      args.SourceVersion.CreateTime,
			RevisionMessage: args.SourceVersion.Message,
		}
	} else {
		metadata.Revision = args.PushRevision
		metadata.SourceCommit = args.PushRevision.Revision
	}
	repo, err := model.FindRepository(args.DownstreamProject.Id)
	if err != nil {
		return metadata, errors.Wrap(err, "finding most recent revision")
	}
	if repo == nil {
		return metadata, errors.Errorf("repo '%s' not found", args.DownstreamProject.Id)
	}
	metadata.Revision.Revision = repo.LastRevision
	var author *user.DBUser
	if args.SourceVersion != nil {
		author, err = user.FindOneById(args.SourceVersion.AuthorID)
		if err != nil {
			return metadata, errors.Wrapf(err, "finding version author '%s'", args.SourceVersion.AuthorID)
		}
	}
	if author != nil {
		metadata.Revision.AuthorGithubUID = author.Settings.GithubUser.UID
	}

	return metadata, nil
}

func makeDownstreamProjectFromFile(ctx context.Context, ref model.ProjectRef, file string) (model.ProjectInfo, error) {
	opts := model.GetProjectOpts{
		Ref:        &ref,
		RemotePath: file,
		Revision:   ref.Branch,
	}
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return model.ProjectInfo{}, errors.Wrap(err, "getting Evergreen settings")
	}
	opts.Token, err = settings.GetGithubOauthToken()
	if err != nil {
		return model.ProjectInfo{}, err
	}

	return model.GetProjectFromFile(context.Background(), opts)
}
