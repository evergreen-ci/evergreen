package command

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/shlex"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitPush(t *testing.T) {
	token := "0123456789"
	c := gitPush{
		Directory: "src",
		Token:     token,
	}
	comm := client.NewMock("http://localhost.com")
	conf := &internal.TaskConfig{
		Task:       task.Task{},
		ProjectRef: model.ProjectRef{Branch: "main"},
		Expansions: util.Expansions{},
	}
	logger, err := comm.GetLoggerProducer(context.Background(), client.TaskData{}, nil)
	require.NoError(t, err)

	assert.Equal(t, conf.GetCloneMethod(), evergreen.CloneMethodOAuth)

	var splitCommand []string
	for name, test := range map[string]func(*testing.T){
		"Execute": func(*testing.T) {
			manager := &mock.Manager{}
			c.base.jasper = manager
			manager.Create = func(opts *options.Create) mock.Process {
				_, err = opts.Output.Output.Write([]byte("abcdef01345"))
				assert.NoError(t, err)
				proc := mock.Process{}
				proc.ProcInfo.Options = *opts
				return proc
			}
			patch := &patch.Patch{
				Patches: []patch.ModulePatch{
					{
						ModuleName: "",
						PatchSet: patch.PatchSet{
							Summary: []thirdparty.Summary{
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
			comm.GetTaskPatchResponse = patch

			ctx := context.Background()
			assert.NoError(t, c.Execute(ctx, comm, logger, conf))

			commands := []string{
				"git checkout main",
				"git push origin refs/heads/main",
			}

			assert.Len(t, manager.Procs, len(commands))
			for i, proc := range manager.Procs {
				args := proc.(*mock.Process).ProcInfo.Options.Args
				splitCommand, err = shlex.Split(commands[i])
				assert.NoError(t, err)
				assert.Equal(t, splitCommand, args)
			}
		},
		"PushPatch": func(*testing.T) {
			ctx := context.Background()
			var jpm jasper.Manager
			jpm, err = jasper.NewSynchronizedManager(false)
			require.NoError(t, err)
			c.base.jasper = jpm
			c.DryRun = true

			repoDir := t.TempDir()
			require.NoError(t, os.WriteFile(path.Join(repoDir, "test1.txt"), []byte("test1"), 0644))
			require.NoError(t, os.WriteFile(path.Join(repoDir, "test2.txt"), []byte("test2"), 0644))

			// create repo
			createRepoCommands := []string{
				`git init`,
				`git add --all`,
				`git -c "user.name=baxterthehacker" -c "user.email=baxter@thehacker.com" commit -m "testing..."`,
			}
			cmd := jpm.CreateCommand(ctx).Directory(repoDir).Append(createRepoCommands...)
			require.NoError(t, cmd.Run(ctx))

			require.NoError(t, os.WriteFile(path.Join(repoDir, "test3.txt"), []byte("test3"), 0644))
			toApplyCommands := []string{
				`git rm test1.txt`,
				`git add test3.txt`,
				`git -c "user.name=baxterthehacker" -c "user.email=baxter@thehacker.com" commit -m "changes to push"`,
			}
			cmd = jpm.CreateCommand(ctx).Directory(repoDir).Append(toApplyCommands...)
			require.NoError(t, cmd.Run(ctx))

			params := pushParams{
				directory: repoDir,
				branch:    "main",
			}
			assert.NoError(t, c.pushPatch(ctx, logger, params))

			stdout := noopWriteCloser{&bytes.Buffer{}}
			cmd = jpm.CreateCommand(ctx).Directory(repoDir).Append(`git diff-tree --no-commit-id --name-only -r HEAD`).SetOutputWriter(stdout)
			assert.NoError(t, cmd.Run(ctx))

			filesChanged := strings.TrimSpace(stdout.String())
			filesChangedSlice := strings.Split(strings.Replace(filesChanged, "\r\n", "\n", -1), "\n")
			require.Len(t, filesChangedSlice, 2)
			assert.Equal(t, "test1.txt", filesChangedSlice[0])
			assert.Equal(t, "test3.txt", filesChangedSlice[1])
		},
		"PushPatchMock": func(*testing.T) {
			manager := &mock.Manager{}
			manager.Create = func(opts *options.Create) mock.Process {
				_, err = opts.Output.Error.Write([]byte(fmt.Sprintf("The key: %s", token)))
				assert.NoError(t, err)
				proc := mock.Process{}
				proc.ProcInfo.Options = *opts
				return proc
			}
			c.base.jasper = manager
			params := pushParams{
				directory: c.Directory,
				branch:    "main",
				token:     token,
			}

			assert.NoError(t, c.pushPatch(context.Background(), logger, params))
			commands := []string{
				"git push origin refs/heads/main",
			}
			require.Len(t, manager.Procs, len(commands))
			for i, proc := range manager.Procs {
				args := proc.(*mock.Process).ProcInfo.Options.Args
				splitCommand, err = shlex.Split(commands[i])
				assert.NoError(t, err)
				assert.Equal(t, splitCommand, args)
			}

			assert.NoError(t, logger.Close())
			lines := comm.GetTaskLogs("")
			assert.Equal(t, "The key: [redacted oauth token]", lines[len(lines)-1].Data)
		},
		"RevParse": func(*testing.T) {
			manager := &mock.Manager{}
			c.base.jasper = manager
			_, err = c.revParse(context.Background(), conf, logger, "main@{upstream}")
			assert.NoError(t, err)
			commands := []string{"git rev-parse main@{upstream}"}

			require.Len(t, manager.Procs, len(commands))
			for i, proc := range manager.Procs {
				args := proc.(*mock.Process).ProcInfo.Options.Args
				splitCommand, err = shlex.Split(commands[i])
				assert.NoError(t, err)
				assert.Equal(t, splitCommand, args)
			}
		},
	} {
		c.DryRun = false
		t.Run(name, test)
	}
}
