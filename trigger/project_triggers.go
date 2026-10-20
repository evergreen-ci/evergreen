package trigger

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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

	projectInfo.Ref = &args.DownstreamProject

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
	upstreamModuleIdx, err := findUpstreamModule(moduleList, upstreamProject)
	if err != nil {
		return nil, err
	}
	if args.TriggerType == model.ProjectTriggerLevelPush {
		if upstreamModuleIdx >= 0 {
			metadata.SourceModule = &model.SourceModule{
				Name:   moduleList[upstreamModuleIdx].Name,
				Owner:  upstreamProject.Owner,
				Repo:   upstreamProject.Repo,
				Branch: upstreamProject.Branch,
			}
		} else {
			grip.Warning(ctx, message.Fields{
				"message":               "push trigger upstream project does not match any module in downstream config",
				"source":                "project-triggers",
				"downstream_project_id": args.DownstreamProject.Id,
				"config_file":           args.ConfigFile,
				"upstream_project_id":   upstreamProject.Id,
				"upstream_owner":        upstreamProject.Owner,
				"upstream_repo":         upstreamProject.Repo,
				"upstream_branch":       upstreamProject.Branch,
				"upstream_commit":       metadata.SourceCommit,
			})
		}
	}

	v, err := repotracker.CreateVersionFromConfig(context.WithoutCancel(ctx), &projectInfo, metadata, false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating version")
	}
	if args.SourceVersion != nil {
		if err = args.SourceVersion.AddSatisfiedTrigger(ctx, args.DefinitionID); err != nil {
			return nil, err
		}
	}
	if upstreamModuleIdx >= 0 {
		if args.TriggerType == model.ProjectTriggerLevelPush {
			moduleList[upstreamModuleIdx].Ref = metadata.SourceCommit
		}
		if _, err = model.CreateManifest(ctx, v, moduleList, projectInfo.Ref); err != nil {
			return nil, errors.WithStack(err)
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

func findUpstreamModule(modules model.ModuleList, upstreamProject *model.ProjectRef) (int, error) {
	for i, module := range modules {
		owner, repo, err := module.GetOwnerAndRepo()
		if err != nil {
			return -1, errors.Wrapf(err, "getting owner and repo for '%s'", module.Name)
		}
		if owner == upstreamProject.Owner && repo == upstreamProject.Repo && module.Branch == upstreamProject.Branch {
			return i, nil
		}
	}
	return -1, nil
}

func getMetadataFromArgs(ctx context.Context, args ProcessorArgs) (model.VersionMetadata, error) {
	metadata := model.VersionMetadata{
		SourceVersion:       args.SourceVersion,
		Activate:            !args.UnscheduleDownstreamVersions,
		TriggerID:           args.TriggerID,
		TriggerType:         args.TriggerType,
		EventID:             args.EventID,
		TriggerDefinitionID: args.DefinitionID,
		Alias:               args.Alias,
	}
	var ingestTime time.Time
	if args.SourceVersion != nil {
		metadata.Revision = model.Revision{
			Author:          args.SourceVersion.Author,
			AuthorEmail:     args.SourceVersion.AuthorEmail,
			CreateTime:      args.SourceVersion.CreateTime,
			RevisionMessage: args.SourceVersion.Message,
		}
		ingestTime = args.SourceVersion.IngestTime

		author, err := user.FindOneById(ctx, args.SourceVersion.AuthorID)
		if err != nil {
			return metadata, errors.Wrapf(err, "finding version author '%s'", args.SourceVersion.AuthorID)
		}
		if author != nil {
			metadata.Revision.AuthorGithubUID = author.Settings.GithubUser.UID
		}
	} else {
		metadata.Revision = args.PushRevision
		metadata.SourceCommit = args.PushRevision.Revision
		ingestTime = args.PushIngestTime
	}
	metadata.IngestTime = ingestTime

	selectionTime := ingestTime
	if utility.IsZeroTime(selectionTime) {
		selectionTime = metadata.Revision.CreateTime
	}
	repoRevision, err := model.FindLatestRepositoryRevisionByIngestTime(ctx, args.DownstreamProject.Owner, args.DownstreamProject.Repo, args.DownstreamProject.Branch, selectionTime)
	if err != nil {
		return metadata, errors.Wrapf(err, "finding latest repository revision by ingest time for '%s'", args.DownstreamProject.Id)
	}
	if repoRevision != nil {
		metadata.Revision.Revision = repoRevision.Revision
	} else {
		opts := &github.CommitsListOptions{
			SHA:   args.DownstreamProject.Branch,
			Until: selectionTime,
			ListOptions: github.ListOptions{
				Page:    0,
				PerPage: 1,
			},
		}

		branchCommits, _, err := thirdparty.GetGithubCommits(ctx, args.DownstreamProject.Owner, args.DownstreamProject.Repo, opts)
		if err != nil {
			return metadata, errors.Wrapf(err, "fetching most recent revision for '%s'", args.DownstreamProject.Id)
		}
		if len(branchCommits) == 0 {
			return metadata, errors.Errorf("no commits found for '%s'", args.DownstreamProject.Id)
		}
		metadata.Revision.Revision = branchCommits[0].GetSHA()
	}

	return metadata, nil
}

func makeDownstreamProjectFromFile(ctx context.Context, ref model.ProjectRef, file string) (model.ProjectInfo, error) {
	opts := model.GetProjectOpts{
		Ref:          &ref,
		RemotePath:   file,
		Revision:     ref.Branch,
		ReadFileFrom: model.ReadFromGithub,
	}
	return model.GetProjectFromFile(ctx, opts, nil)
}
