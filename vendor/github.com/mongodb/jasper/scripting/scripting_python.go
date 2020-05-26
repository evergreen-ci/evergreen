package scripting

import (
	"bufio"
	"context"
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
)

type pythonEnvironment struct {
	opts *options.ScriptingPython

	isConfigured bool
	cachedHash   string
	manager      jasper.Manager
}

func (e *pythonEnvironment) ID() string { e.cachedHash = e.opts.ID(); return e.cachedHash }

func (e *pythonEnvironment) Setup(ctx context.Context) error {
	if e.isConfigured && e.cachedHash == e.opts.ID() {
		return nil
	}

	e.cachedHash = e.opts.ID()
	venvpy := e.opts.Interpreter()

	cmd := e.manager.CreateCommand(ctx).AppendArgs(e.opts.InterpreterBinary, "-m", e.venvMod(), e.opts.VirtualEnvPath).Environment(e.opts.Environment)

	if e.opts.RequirementsPath != "" {
		args := []string{venvpy, "-m", "pip", "install", "-r", e.opts.RequirementsPath}
		if e.opts.UpdatePackages {
			args = append(args, "--upgrade")
		}
		cmd.Add(args)
	}

	for _, pkg := range e.opts.Packages {
		args := []string{venvpy, "-m", "pip", "install"}
		if e.opts.UpdatePackages {
			args = append(args, "--update")
		}
		args = append(args, pkg)
		cmd.Add(args)
	}

	cmd.PostHook(func(res error) error {
		if res == nil {
			e.isConfigured = true
		}
		return nil
	})

	return cmd.SetOutputOptions(e.opts.Output).Run(ctx)
}

func (e *pythonEnvironment) venvMod() string {
	if e.opts.LegacyPython {
		return "virtualenv"
	}
	return "venv"
}

func (e *pythonEnvironment) Run(ctx context.Context, args []string) error {
	return e.manager.CreateCommand(ctx).Add(append([]string{e.opts.Interpreter()}, args...)).Run(ctx)
}

func (e *pythonEnvironment) RunScript(ctx context.Context, script string) error {
	scriptChecksum := fmt.Sprintf("%x", sha1.Sum([]byte(script)))

	wo := options.WriteFile{
		Path:    filepath.Join(e.opts.VirtualEnvPath, "tmp", strings.Join([]string{e.manager.ID(), scriptChecksum}, "-")+".py"),
		Content: []byte(script),
	}
	if err := e.manager.WriteFile(ctx, wo); err != nil {
		return errors.Wrap(err, "problem writing script file")
	}

	return e.manager.CreateCommand(ctx).Environment(e.opts.Environment).SetOutputOptions(e.opts.Output).AppendArgs(e.opts.Interpreter(), wo.Path).Run(ctx)
}

func (e *pythonEnvironment) Build(ctx context.Context, dir string, args []string) (string, error) {
	output := &util.LocalBuffer{}

	err := e.manager.CreateCommand(ctx).Directory(dir).RedirectErrorToOutput(true).Environment(e.opts.Environment).
		Add(append([]string{e.opts.Interpreter(), "setup.py", "bdist_wheel"}, args...)).
		SetOutputWriter(output).SetOutputOptions(e.opts.Output).Run(ctx)
	if err != nil {
		return "", errors.WithStack(err)
	}

	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		ln := scanner.Text()
		if strings.Contains("whl", ln) {
			parts := strings.Split(ln, " ")
			if len(parts) > 2 {
				return strings.Trim(parts[1], "'"), nil
			}
			break
		}
	}
	return "", nil
}

func (e *pythonEnvironment) Cleanup(ctx context.Context) error {
	switch mgr := e.manager.(type) {
	case remote:
		return errors.Wrapf(mgr.CreateCommand(ctx).SetOutputOptions(e.opts.Output).AppendArgs("rm", "-rf", e.opts.VirtualEnvPath).Run(ctx),
			"problem removing remote python environment '%s'", e.opts.VirtualEnvPath)
	default:
		return errors.Wrapf(os.RemoveAll(e.opts.VirtualEnvPath),
			"problem removing local python environment '%s'", e.opts.VirtualEnvPath)
	}
}

func (e *pythonEnvironment) Test(ctx context.Context, dir string, tests ...TestOptions) ([]TestResult, error) {
	out := make([]TestResult, len(tests))

	catcher := grip.NewBasicCatcher()
	for idx, t := range tests {
		startAt := time.Now()
		args := []string{e.opts.Interpreter(), "-m", "pytest"}
		if t.Count > 0 {
			args = append(args, fmt.Sprintf("--count=%d", t.Count))
		}
		if t.Timeout > 0 {
			args = append(args, fmt.Sprintf("--timeout='%s'", t.Timeout.String()))
		}
		if t.Pattern != "" {
			args = append(args, "-k", t.Pattern)
		}

		args = append(args, t.Args...)

		err := e.manager.CreateCommand(ctx).Directory(dir).Environment(e.opts.Environment).SetOutputOptions(e.opts.Output).Add(args).Run(ctx)

		catcher.Wrapf(err, "python test %s", t)

		out[idx] = t.getResult(ctx, err, startAt)
	}

	return out, catcher.Resolve()
}
