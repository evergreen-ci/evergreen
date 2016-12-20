package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

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
	)

	flag.StringVar(&arch, "goarch", runtime.GOARCH, "target architecture (GOARCH)")
	flag.StringVar(&system, "goos", runtime.GOOS, "target system (GOOS)")
	flag.StringVar(&directory, "directory", "", "output directory")
	flag.StringVar(&source, "source", "", "path to source file")
	flag.StringVar(&ldFlags, "ldflags", "", "specify any ldflags to pass to go build")
	flag.StringVar(&buildName, "buildName", "", "use GOOS_ARCH to specify target platform")
	flag.StringVar(&goBin, "goBinary", "go", "specify path to go binary")
	flag.StringVar(&output, "output", "", "specify the name of executable")
	flag.Parse()

	if buildName != "" {
		parts := strings.Split(buildName, "_")

		system = parts[0]
		arch = parts[1]
	} else {
		buildName = fmt.Sprintf("%s_%s", system, arch)
	}
	if runtime.GOOS == "windows" && goBin == "/cygdrive/c/go/bin/go" {
		goBin = `c:\\go\\bin\\go`
	}

	cmd := exec.Command(goBin, "build")

	cmd.Args = append(cmd.Args, "-o", output)
	if ldFlags != "" {
		cmd.Args = append(cmd.Args, "-ldflags=\""+ldFlags+"\"")
	}
	cmd.Args = append(cmd.Args, source)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		"GOPATH=" + strings.Replace(os.Getenv("GOPATH"), `\`, `\\`, -1),
		"GOROOT=" + runtime.GOROOT(),
	}

	if runtime.Compiler != "gccgo" {
		cmd.Env = append(cmd.Env,
			"GOOS="+system,
			"GOARCH="+arch)
	}

	fmt.Println(strings.Join(cmd.Env, " "), strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		fmt.Printf("problem building %s: %v\n", output, err)
		os.Exit(1)
	}
}
