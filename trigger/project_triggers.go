package trigger

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// TriggerDownstreamVersion assumes that you definitely want to create a downstream version
// and will go through the process of version creation given a triggering version
func TriggerDownstreamVersion(args ProcessorArgs) (*model.Version, error) {
	if args.ConfigFile != "" && args.Command != "" {
		return nil, errors.New("cannot specify both a file and command")
	}
	if args.SourceVersion == nil {
		return nil, errors.Errorf("unable to find source version in project %s", args.DownstreamProject.Id)
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
	var proj *model.Project
	var pp *model.ParserProject
	if args.ConfigFile != "" {
		proj, pp, err = makeDownstreamProjectFromFile(args.DownstreamProject, args.ConfigFile)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else if args.Command != "" {
		proj, pp, err = makeDownstreamProjectFromCommand(args.DownstreamProject.Id, args.Command, args.GenerateFile)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else {
		return nil, errors.New("must specify a file or command to define downstream project config")
	}

	// create version
	projectInfo := &model.ProjectInfo{
		Ref:                 &args.DownstreamProject,
		Project:             proj,
		IntermediateProject: pp,
	}
	v, err := repotracker.CreateVersionFromConfig(context.Background(), projectInfo, metadata, false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error creating version")
	}
	err = args.SourceVersion.AddSatisfiedTrigger(args.DefinitionID)
	if err != nil {
		return nil, err
	}
	settings, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error getting evergreen settings")
	}
	upstreamProject, err := model.FindMergedProjectRef(args.SourceVersion.Identifier)
	if err != nil {
		return nil, errors.Wrap(err, "error finding project ref")
	}
	if upstreamProject == nil {
		return nil, errors.Errorf("project %s not found", args.SourceVersion.Identifier)
	}
	for _, module := range proj.Modules {
		owner, repo := module.GetRepoOwnerAndName()
		if owner == upstreamProject.Owner && repo == upstreamProject.Repo && module.Branch == upstreamProject.Branch {
			_, err = repotracker.CreateManifest(*v, proj, upstreamProject, settings)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			break
		}
	}
	err = model.UpdateLastRevision(v.Identifier, v.Revision)
	if err != nil {
		return nil, errors.Wrap(err, "error updating last revision")
	}
	err = repotracker.AddBuildBreakSubscriptions(v, &args.DownstreamProject)
	if err != nil {
		return nil, errors.Wrap(err, "error adding build break subscriptions")
	}
	_, err = model.DoProjectActivation(args.DownstreamProject.Id, time.Now())
	if err != nil {
		return nil, errors.Wrapf(err, "error activating project %s", args.DownstreamProject.Id)
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

func makeDownstreamProjectFromFile(ref model.ProjectRef, file string) (*model.Project, *model.ParserProject, error) {
	opts := model.GetProjectOpts{
		Ref:        &ref,
		RemotePath: file,
		Revision:   ref.Branch,
	}
	settings, err := evergreen.GetConfig()
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting evergreen settings")
	}
	opts.Token, err = settings.GetGithubOauthToken()
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	return model.GetProjectFromFile(ctx, opts)
}

func makeDownstreamProjectFromCommand(identifier, command, generateFile string) (*model.Project, *model.ParserProject, error) {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return nil, nil, errors.Wrap(err, "error retrieving config")
	}
	bvtName := "generate-config"
	fullProject := &model.Project{
		Identifier: identifier,
		Tasks: []model.ProjectTask{
			{
				Name: "generate-config",
				Commands: []model.PluginCommandConf{
					{
						Command: "git.get_project",
						Type:    evergreen.CommandTypeSetup,
						Params: map[string]interface{}{
							"directory": "${workdir}/src",
						},
					},
					{
						Command: "subprocess.exec",
						Params: map[string]interface{}{
							"working_dir": "src",
							"command":     command,
						},
					},
					{
						Command: "generate.tasks",
						Params: map[string]interface{}{
							"files": []string{generateFile},
						},
					},
				},
			},
		},
		BuildVariants: model.BuildVariants{
			{
				Name:        "generate",
				DisplayName: "generate",
				RunOn:       []string{settings.Triggers.GenerateTaskDistro},
				Tasks: []model.BuildVariantTaskUnit{
					{Name: bvtName},
				},
			},
		},
	}
	pp := &model.ParserProject{
		Identifier: utility.ToStringPtr(identifier),
	}

	pp.AddTask(fullProject.Tasks[0].Name, fullProject.Tasks[0].Commands)
	pp.AddBuildVariant(fullProject.BuildVariants[0].Name, fullProject.BuildVariants[0].DisplayName, settings.Triggers.GenerateTaskDistro, nil, []string{bvtName})
	return fullProject, pp, nil
}
