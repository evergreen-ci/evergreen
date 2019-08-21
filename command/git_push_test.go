package command

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/shlex"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitPush(t *testing.T) {
	token := "0123456789"
	c := gitPush{
		Directory:      "src",
		CommitterName:  "octocat",
		CommitterEmail: "octocat@github.com",
		Token:          token,
	}
	comm := client.NewMock("http://localhost.com")
	conf := &model.TaskConfig{
		Task:       &task.Task{},
		ProjectRef: &model.ProjectRef{Branch: "master"},
		Distro:     &distro.Distro{CloneMethod: distro.CloneMethodOAuth},
		Expansions: &util.Expansions{},
	}
	logger, err := comm.GetLoggerProducer(context.Background(), client.TaskData{}, nil)
	require.NoError(t, err)

	var splitCommand []string
	for name, test := range map[string]func(*testing.T){
		"Execute": func(*testing.T) {
			manager := &jasper.MockManager{}
			c.base.jasper = manager
			manager.Create = func(opts *jasper.CreateOptions) jasper.MockProcess {
				_, err = opts.Output.Output.Write([]byte("abcdef01345"))
				assert.NoError(t, err)
				proc := jasper.MockProcess{}
				proc.ProcInfo.Options = *opts
				return proc
			}
			patch := &patch.Patch{
				Patches: []patch.ModulePatch{
					{
						ModuleName: "",
						PatchSet: patch.PatchSet{
							Summary: []patch.Summary{
								{
									Name: "hello.txt",
								},
							},
						},
					},
				},
				Githash:     "abcdef01345",
				Description: "testing 123",
			}

			ctx := context.Background()
			ctx = context.WithValue(ctx, "patch", patch)
			assert.NoError(t, c.Execute(ctx, comm, logger, conf))

			commands := []string{
				"git checkout master",
				"git rev-parse HEAD",
				`git -c "user.name=octocat" -c "user.email=octocat@github.com" commit --file - --author="evergreen <evergreen@mongodb.com>"`,
				"git push origin master",
			}

			require.Len(t, manager.Procs, len(commands))
			for i, proc := range manager.Procs {
				args := proc.(*jasper.MockProcess).ProcInfo.Options.Args
				splitCommand, err = shlex.Split(commands[i])
				assert.NoError(t, err)
				assert.Equal(t, splitCommand, args)
			}
		},
		"PushPatch": func(*testing.T) {
			manager := &jasper.MockManager{}
			manager.Create = func(opts *jasper.CreateOptions) jasper.MockProcess {
				_, err = opts.Output.Error.Write([]byte(fmt.Sprintf("The key: %s", token)))
				assert.NoError(t, err)
				proc := jasper.MockProcess{}
				proc.ProcInfo.Options = *opts
				return proc
			}
			c.base.jasper = manager
			params := pushParams{
				directory:     c.Directory,
				authorName:    "baxterthehacker",
				authorEmail:   "baxter@thehacker.com",
				commitMessage: "testing 123",
				branch:        "master",
				token:         token,
			}

			assert.NoError(t, c.pushPatch(context.Background(), logger, params))
			commands := []string{
				`git -c "user.name=octocat" -c "user.email=octocat@github.com" commit --file - --author="baxterthehacker <baxter@thehacker.com>"`,
				"git push origin master",
			}
			require.Len(t, manager.Procs, len(commands))
			for i, proc := range manager.Procs {
				args := proc.(*jasper.MockProcess).ProcInfo.Options.Args
				splitCommand, err = shlex.Split(commands[i])
				assert.NoError(t, err)
				assert.Equal(t, splitCommand, args)
			}

			assert.NoError(t, logger.Close())
			msgs := comm.GetMockMessages()[""]
			assert.Equal(t, "The key: [redacted oauth token]", msgs[len(msgs)-1].Message)
		},
		"RevParse": func(*testing.T) {
			manager := &jasper.MockManager{}
			c.base.jasper = manager
			_, err = c.revParse(context.Background(), conf, logger, "HEAD")
			assert.NoError(t, err)
			commands := []string{"git rev-parse HEAD"}

			require.Len(t, manager.Procs, len(commands))
			for i, proc := range manager.Procs {
				args := proc.(*jasper.MockProcess).ProcInfo.Options.Args
				splitCommand, err = shlex.Split(commands[i])
				assert.NoError(t, err)
				assert.Equal(t, splitCommand, args)
			}
		},
	} {
		t.Run(name, test)
	}
}
