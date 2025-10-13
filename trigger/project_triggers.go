package trigger

import (
	"context"
	"runtime/debug"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// TriggerDownstreamVersion assumes that you definitely want to create a downstream version
// and will go through the process of version creation given a triggering version.
// If the trigger is a push event, the triggering version will be nonexistent.
// kim: NOTE: this is the main logic to create a downstream version for a push
// trigger. Task/BV triggers are handled on task/BV finish events.
// kim: TODO: figure out where exactly this gets triggered on commit push. I
// would expect it to happen somewhere in the repotracker code, but I don't see
// it being triggered there from quick inspection.
func TriggerDownstreamVersion(ctx context.Context, args ProcessorArgs) (*model.Version, error) {
	grip.DebugWhen(args.TriggerType == model.ProjectTriggerLevelPush, message.Fields{
		"message": "kim: triggering downstream version for push event",
		"args":    args,
		"stack":   string(debug.Stack()),
	})
	if args.SourceVersion == nil && args.TriggerType != model.ProjectTriggerLevelPush {
		// kim: NOTE: all this means is that task/BV-level triggers must have a
		// source version that triggered the downstream version creation.
		return nil, errors.Errorf("unable to find source version in downstream project '%s'", args.DownstreamProject.Id)
	}

	// propagate version metadata to the downstream version
	metadata, err := getMetadataFromArgs(ctx, args)
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
	// kim: NOTE: this implies that it'll only create one version. If downstream
	// triggers for push are in fact created by
	// TriggerDownstreamProjectsForPush (which does appear to be the case from
	// staging), then that means it will miss older versions.
	projectInfo.Ref = &args.DownstreamProject
	v, err := repotracker.CreateVersionFromConfig(context.Background(), &projectInfo, metadata, false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating version")
	}
	if args.SourceVersion != nil {
		if err = args.SourceVersion.AddSatisfiedTrigger(ctx, args.DefinitionID); err != nil {
			return nil, err
		}
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
	upstreamProject, err := model.FindMergedProjectRef(ctx, projectID, versionID, true)
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
			_, err = model.CreateManifest(ctx, v, moduleList, projectInfo.Ref)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			break
		}
	}
	err = model.UpdateLastRevision(ctx, v.Identifier, v.Revision)
	if err != nil {
		return nil, errors.Wrap(err, "updating last revision")
	}
	err = repotracker.AddBuildBreakSubscriptions(ctx, v, &args.DownstreamProject)
	if err != nil {
		return nil, errors.Wrap(err, "adding build break subscriptions")
	}
	return v, nil
}

func getMetadataFromArgs(ctx context.Context, args ProcessorArgs) (model.VersionMetadata, error) {
	metadata := model.VersionMetadata{
		SourceVersion: args.SourceVersion,
		// kim: NOTE: this is not set in mms-alert-deploy-triggers, so it should
		// always activate the downstream version once it's created. However,
		// the issue looks like the downstream version isn't created at all, so
		// the issue is liekly not with the version activation logic.
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

		author, err := user.FindOneByIdContext(ctx, args.SourceVersion.AuthorID)
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
	repo, err := model.FindRepository(ctx, args.DownstreamProject.Id)
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
		Identifier: ref.Identifier,
	}
	return model.GetProjectFromFile(ctx, opts)
}
