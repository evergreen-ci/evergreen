package trigger

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/repotracker"
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
	metadata, err := getMetadataFromArgs(args)
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
	// Since push triggers have no source version (unlike build and task level triggers), we need to
	// extract the project ID from the trigger definition's project ID, which is populated in the TriggerID field
	// for push triggers.
	var versionID, projectID string
	if args.TriggerType == model.ProjectTriggerLevelPush {
		projectID = args.TriggerID
	} else {
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
	moduleList := projectInfo.Project.Modules
	for i, module := range moduleList {
		owner, repo, err := module.GetOwnerAndRepo()
		if err != nil {
			return nil, errors.Wrapf(err, "getting owner and repo for '%s'", module.Name)
		}

		if owner == upstreamProject.Owner && repo == upstreamProject.Repo && module.Branch == upstreamProject.Branch {
			if args.TriggerType == model.ProjectTriggerLevelPush {
				moduleList[i].Ref = metadata.SourceCommit
			}
			_, err = model.CreateManifest(v, moduleList, projectInfo.Ref, settings)
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

func getMetadataFromArgs(args ProcessorArgs) (model.VersionMetadata, error) {
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

		author, err := user.FindOneById(args.SourceVersion.AuthorID)
		if err != nil {
			return metadata, errors.Wrapf(err, "finding version author '%s'", args.SourceVersion.AuthorID)
		}
		if author != nil {
			metadata.Revision.AuthorGithubUID = author.Settings.GithubUser.UID
		}
	} else {
		metadata.Revision = args.PushRevision
		metadata.SourceCommit = args.PushRevision.Revision
	}
	repo, err := model.FindRepository(args.DownstreamProject.Id)
	if err != nil {
		return metadata, errors.Wrapf(err, "finding most recent revision for '%s'", args.DownstreamProject.Id)
	}
	if repo == nil {
		return metadata, errors.Errorf("repo '%s' not found", args.DownstreamProject.Id)
	}
	metadata.Revision.Revision = repo.LastRevision

	return metadata, nil
}

func makeDownstreamProjectFromFile(ctx context.Context, ref model.ProjectRef, file string) (model.ProjectInfo, error) {
	opts := model.GetProjectOpts{
		Ref:        &ref,
		RemotePath: file,
		Revision:   ref.Branch,
	}
	return model.GetProjectFromFile(ctx, opts)
}
