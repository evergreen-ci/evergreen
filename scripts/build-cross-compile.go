package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func supportsRaceDetector(arch, system string) bool {
	if arch != "amd64" {
		return false
	}

	for _, platform := range []string{"freebsd", "linux", "darwin", "windows"} {
		if runtime.GOOS == platform && system == platform {
			return true
		}
	}

	return false
}

func main() {
	var (
		arch      string
		system    string
		directory string
		source    string
		ldFlags   string
		buildName string
		output    string
		goBin     string
		race      bool

		defaultArch   string
		defaultSystem string
	)

	defaultArch = os.Getenv("GOARCH")
	if defaultArch == "" {
		defaultArch = runtime.GOARCH
	}
	defaultSystem = os.Getenv("GOOS")
	if defaultSystem == "" {
		defaultSystem = runtime.GOOS
	}

	flag.StringVar(&arch, "goarch", defaultArch, "target architecture (GOARCH)")
	flag.StringVar(&system, "goos", defaultSystem, "target system (GOOS)")
	flag.StringVar(&directory, "directory", "", "output directory")
	flag.StringVar(&source, "source", "", "path to source file")
	flag.StringVar(&ldFlags, "ldflags", "", "specify any ldflags to pass to go build")
	flag.StringVar(&buildName, "buildName", "", "use GOOS_ARCH to specify target platform")
	flag.StringVar(&goBin, "goBinary", "go", "specify path to go binary")
	flag.StringVar(&output, "output", "", "specify the name of executable")
	flag.BoolVar(&race, "race", false, "specify to enable the race detector")
	flag.Parse()

	if buildName != "" {
		parts := strings.Split(buildName, "_")

		system = parts[0]
		arch = parts[1]
	} else {
		buildName = fmt.Sprintf("%s_%s", system, arch)
	}

	cmd := exec.Command(goBin, "build")
	ldf := fmt.Sprintf("-ldflags=%s", ldFlags)
	ldfQuoted := fmt.Sprintf("-ldflags=\"%s\"", ldFlags)
	cmd.Args = append(cmd.Args, ldf)

	cmd.Env = []string{
		"PATH=" + strings.Replace(os.Getenv("PATH"), `\`, `\\`, -1),
		"GOPATH=" + strings.Replace(os.Getenv("GOPATH"), `\`, `\\`, -1),
		"GOROOT=" + runtime.GOROOT(),
	}

	if race && supportsRaceDetector(arch, system) {
		cmd.Args = append(cmd.Args, "-race")
		cmd.Env = append(cmd.Env, "CGO_ENABLED=1")
	}

	if runtime.Compiler != "gccgo" {
		cmd.Env = append(cmd.Env,
			"GOOS="+system,
			"GOARCH="+arch)
	}

	cmd.Args = append(cmd.Args, "-o", output)
	cmd.Args = append(cmd.Args, source)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmdString := strings.Join(cmd.Args, " ")
	fmt.Println(strings.Join(cmd.Env[1:], " "), strings.Replace(cmdString, ldf, ldfQuoted, -1))
	if err := cmd.Run(); err != nil {
		fmt.Printf("problem building %s: %v\n", output, err)
		os.Exit(1)
	}
}
