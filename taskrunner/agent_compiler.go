package taskrunner

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model/distro"
	"io/ioutil"
	"path/filepath"
	"strings"
)

func init() {
	// find and store the path to the goxc executable.
	// TODO: this shouldn't be necessary, goxc should be compiled,
	// added to $PATH, and called directly rather than using 'go run'
	evgHome := evergreen.FindEvergreenHome()
	goxc = filepath.Join(evgHome, "src/github.com/laher/goxc/goxc.go")
}

// AgentCompiler controls cross-compilation of the agent package.
type AgentCompiler interface {
	// Compile the agent package into the specified directory
	// Takes in the sourceDir (where the agent package lives) and the desired
	// destination directory for the built binaries.
	// Returns an error if the compilation fails.
	Compile(sourceDir string, destDir string) error

	// Given a distro, return the correct subpath to appropriate executable
	// within the directory where the compiler cross-compiles all of the
	// executables
	ExecutableSubPath(distroId string) (string, error)
}

// GoxcAgentCompiler uses goxc as the cross-compiler of choice.
type GoxcAgentCompiler struct {
	*evergreen.Settings
}

// TODO native gc cross-compiler for Go 1.5

var (
	// These variables determine what platforms and architectures we build the
	// agent for.  They are passed as arguments to the goxc binary.
	AgentOSTargets   = "windows darwin linux solaris"
	AgentArchTargets = "amd64 386"

	// The location of the goxc file
	goxc string
)

// Compile cross-compiles the specified package into the specified destination dir,
// using goxc as the cross-compiler.
func (self *GoxcAgentCompiler) Compile(sourceDir string, destDir string) error {

	buildAgentStr := fmt.Sprintf(
		`go run %v -tasks-=go-vet,go-test,archive,rmbin -d %v -os="%v" -arch="%v"`,
		goxc, destDir, AgentOSTargets, AgentArchTargets)

	buildAgentCmd := &command.LocalCommand{
		CmdString:        buildAgentStr,
		Stdout:           ioutil.Discard, // TODO: change to real logging
		Stderr:           ioutil.Discard,
		WorkingDirectory: sourceDir,
	}

	// build the agent
	if err := buildAgentCmd.Run(); err != nil {
		return fmt.Errorf("error building agent: %v", err)
	}

	return nil
}

// ExecutableSubPath returns the directory containing the compiled agents.
func (self *GoxcAgentCompiler) ExecutableSubPath(id string) (string, error) {

	// get the full distro info, so we can figure out the architecture
	d, err := distro.FindOne(distro.ById(id))
	if err != nil {
		return "", fmt.Errorf("error finding distro %v: %v", id, err)
	}

	mainName := "main"
	if strings.HasPrefix(d.Arch, "windows") {
		mainName = "main.exe"
	}

	return filepath.Join("snapshot", d.Arch, mainName), nil
}
