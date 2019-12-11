package scripting

import (
	"context"
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

type roswellEnvironment struct {
	opts *options.ScriptingRoswell

	isConfigured bool
	cachedHash   string
	manager      jasper.Manager
}

func (e *roswellEnvironment) ID() string { e.cachedHash = e.opts.ID(); return e.cachedHash }
func (e *roswellEnvironment) Setup(ctx context.Context) error {
	if e.isConfigured && e.cachedHash == e.opts.ID() {
		return nil
	}

	cmd := e.manager.CreateCommand(ctx).Environment(e.opts.Environment).AddEnv("ROSWELL_HOME", e.opts.Path).
		SetOutputOptions(e.opts.Output).AppendArgs(e.opts.Interpreter(), "install", e.opts.Lisp)
	for _, sys := range e.opts.Systems {
		cmd.AppendArgs(e.opts.Interpreter(), "install", sys)
	}

	cmd.SetHook(func(res error) error {
		if res == nil {
			e.isConfigured = true
		}
		return nil
	})

	return cmd.Run(ctx)
}

func (e *roswellEnvironment) Run(ctx context.Context, forms []string) error {
	ros := []string{
		e.opts.Interpreter(), "run",
	}
	for _, f := range forms {
		ros = append(ros, "-e", f)
	}
	ros = append(ros, "-q")

	return e.manager.CreateCommand(ctx).Environment(e.opts.Environment).AddEnv("ROSWELL_HOME", e.opts.Path).
		SetOutputOptions(e.opts.Output).Add(ros).Run(ctx)
}

func (e *roswellEnvironment) RunScript(ctx context.Context, script string) error {
	scriptChecksum := fmt.Sprintf("%x", sha1.Sum([]byte(script)))
	wo := options.WriteFile{
		Path:    filepath.Join(e.opts.Path, "tmp", strings.Join([]string{e.manager.ID(), scriptChecksum}, "-")+".ros"),
		Content: []byte(script),
	}

	if err := e.manager.WriteFile(ctx, wo); err != nil {
		return errors.Wrap(err, "problem writing file")
	}

	return e.manager.CreateCommand(ctx).Environment(e.opts.Environment).AddEnv("ROSWELL_HOME", e.opts.Path).
		SetOutputOptions(e.opts.Output).AppendArgs(e.opts.Interpreter(), wo.Path).Run(ctx)
}

func (e *roswellEnvironment) Build(ctx context.Context, dir string, args []string) (string, error) {
	err := e.manager.CreateCommand(ctx).Directory(dir).Environment(e.opts.Environment).AddEnv("ROSWELL_HOME", e.opts.Path).
		SetOutputOptions(e.opts.Output).Add(append([]string{e.opts.Interpreter(), "dump", "executable"}, args...)).Run(ctx)
	if err != nil {
		return "", errors.WithStack(err)
	}

	if len(args) >= 1 {
		return strings.TrimRight(args[0], ".ros"), nil
	}

	return "", nil
}

func (e *roswellEnvironment) Cleanup(ctx context.Context) error {
	switch mgr := e.manager.(type) {
	case remote:
		return errors.Wrapf(mgr.CreateCommand(ctx).SetOutputOptions(e.opts.Output).AppendArgs("rm", "-rf", e.opts.Path).Run(ctx),
			"problem removing remote roswell environment '%s'", e.opts.Path)
	default:
		return errors.Wrapf(os.RemoveAll(e.opts.Path),
			"problem removing local roswell environment '%s'", e.opts.Path)
	}
}
