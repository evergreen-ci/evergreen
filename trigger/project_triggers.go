package trigger

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// maxWaterfallHeadMovedRetries bounds how many times a downstream version is
// created again after another version replaced the waterfall head first.
const maxWaterfallHeadMovedRetries = 3

// TriggerDownstreamVersion assumes that you definitely want to create a downstream version
// and will go through the process of version creation given a triggering version.
// If the trigger is a push event, the triggering version will be nonexistent.
// It returns a nil version if the push event's commit is already on the
// downstream waterfall.
func TriggerDownstreamVersion(ctx context.Context, args ProcessorArgs) (*model.Version, error) {
	for attempt := 1; ; attempt++ {
		v, err := triggerDownstreamVersion(ctx, args)
		if !errors.Is(err, model.ErrWaterfallHeadMoved) || attempt > maxWaterfallHeadMovedRetries {
			return v, err
		}
		grip.Info(ctx, message.WrapError(err, message.Fields{
			"message":               "creating downstream version again from the new waterfall head",
			"source":                "project-triggers",
			"downstream_project_id": args.DownstreamProject.Id,
			"trigger_type":          args.TriggerType,
			"attempt":               attempt,
		}))
	}
}

func triggerDownstreamVersion(ctx context.Context, args ProcessorArgs) (*model.Version, error) {
	if args.SourceVersion == nil && args.TriggerType != model.ProjectTriggerLevelPush {
		return nil, errors.Errorf("unable to find source version in downstream project '%s'", args.DownstreamProject.Id)
	}

	// A push-triggered version keeps the revision of the version before it, so
	// it only differs from it by the triggering commit.
	var head *model.WaterfallHead
	var revision string
	var err error
	if args.TriggerType == model.ProjectTriggerLevelPush {
		head, err = model.FindWaterfallHead(ctx, args.DownstreamProject.Id)
		if err != nil {
			return nil, errors.Wrapf(err, "finding waterfall head for downstream project '%s'", args.DownstreamProject.Id)
		}
		revision = head.Revision
	}

	// propagate version metadata to the downstream version
	metadata, err := getMetadataFromArgs(ctx, args, revision)
	if err != nil {
		return nil, err
	}

	if args.ConfigFile == "" {
		return nil, errors.New("must specify a file to define downstream project config")
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

	// Set the ingest time up front so the version and its module commits use the same cutoff.
	if utility.IsZeroTime(metadata.IngestTime) {
		metadata.IngestTime = time.Now()
	}
	resolverOpts := model.ModuleResolverOpts{
		Revision:      metadata.Revision.Revision,
		Requester:     evergreen.TriggerRequester,
		IngestTime:    metadata.IngestTime,
		WaterfallHead: head,
	}
	if args.TriggerType == model.ProjectTriggerLevelPush {
		resolverOpts.TriggeringModuleCommit = &manifest.Module{
			Owner:    upstreamProject.Owner,
			Repo:     upstreamProject.Repo,
			Branch:   upstreamProject.Branch,
			Revision: metadata.SourceCommit,
		}
	}
	resolver := model.NewModuleResolver(&args.DownstreamProject, resolverOpts)
	metadata.ModuleResolver = resolver

	stale, err := resolver.IsTriggeringCommitStale(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "checking if triggering commit is already on the waterfall")
	}
	if stale {
		grip.Info(ctx, message.Fields{
			"message":               "skipping push-triggered version because the downstream waterfall already has the triggering commit or a newer one",
			"source":                "project-triggers",
			"downstream_project_id": args.DownstreamProject.Id,
			"upstream_project_id":   upstreamProject.Id,
			"upstream_commit":       metadata.SourceCommit,
			"waterfall_head":        head.VersionID,
		})
		return nil, nil
	}

	projectInfo, err := makeDownstreamProjectFromFile(ctx, args.DownstreamProject, args.ConfigFile, metadata.Revision.Revision, resolver)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	projectInfo.Ref = &args.DownstreamProject

	if args.TriggerType == model.ProjectTriggerLevelPush {
		triggeringModule, err := resolver.TriggeringModuleName(ctx, projectInfo.Project.Modules)
		if err != nil {
			return nil, errors.Wrap(err, "finding module for triggering commit")
		}
		if triggeringModule != "" {
			metadata.SourceModule = &model.SourceModule{
				Name:   triggeringModule,
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

// getMetadataFromArgs returns the downstream version's metadata. If revision
// is empty, the downstream revision is the latest one as of when the trigger
// fired.
func getMetadataFromArgs(ctx context.Context, args ProcessorArgs, revision string) (model.VersionMetadata, error) {
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
	if revision != "" {
		metadata.Revision.Revision = revision
		return metadata, nil
	}

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

func makeDownstreamProjectFromFile(ctx context.Context, ref model.ProjectRef, file, revision string, resolver *model.ModuleResolver) (model.ProjectInfo, error) {
	opts := model.GetProjectOpts{
		Ref:            &ref,
		RemotePath:     file,
		Revision:       revision,
		ReadFileFrom:   model.ReadFromGithub,
		ModuleResolver: resolver,
	}
	return model.GetProjectFromFile(ctx, opts, nil)
}
