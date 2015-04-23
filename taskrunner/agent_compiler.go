package taskrunner

import (
	"10gen.com/mci"
	"10gen.com/mci/command"
	"10gen.com/mci/model"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
)

// Interface comprising any type meant to cross-compile the
// agent package
type AgentCompiler interface {
	// Compile the agent package into the specified directory
	// Takes in the sourceDir (where the agent package lives) and the desired
	// destination directory for the built binaries.
	// Returns an error if the compilation fails.
	Compile(sourceDir string, destDir string) error

	// Given a distro, return the correct subpath to appropriate executable
	// within the directory where the compiler cross-compiles all of the
	// executables
	ExecutableSubPath(distroName string) (string, error)
}

// Implementation of an AgentCompiler, using goxc as the cross-compiler
// of choice.
type GoxcAgentCompiler struct {
	*mci.MCISettings
}

var (
	// These variables determine what platforms and architectures we build the
	// agent for.  They are passed as arguments to the goxc binary.
	AgentOSTargets   = "windows darwin linux solaris"
	AgentArchTargets = "amd64 386"

	// The location of the goxc file
	goxc string
)

func init() {
	// find and store the path to the goxc executable.
	// TODO: this shouldn't be necessary, goxc should be compiled,
	// added to $PATH, and called directly rather than using 'go run'
	mciHome, err := mci.FindMCIHome()
	if err != nil {
		panic(fmt.Sprintf("error finding mci home while initializing"+
			" compiler: %v", err))
	}
	goxc = filepath.Join(mciHome, "src/github.com/laher/goxc/goxc.go")
}

// Cross-compiles the specified package into the specified destination dir,
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

func (self *GoxcAgentCompiler) ExecutableSubPath(distroName string) (
	string, error) {

	// get the full distro info, so we can figure out the architecture
	distro, err := model.LoadOneDistro(self.MCISettings.ConfigDir, distroName)
	if err != nil {
		return "", fmt.Errorf("error finding distro %v: %v", distroName, err)
	}

	mainName := "main"
	if strings.HasPrefix(distro.Arch, "windows") {
		mainName = "main.exe"
	}

	return filepath.Join("snapshot", distro.Arch, mainName), nil
}
