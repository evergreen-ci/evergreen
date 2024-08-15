package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const (
	macGOOS       = "darwin"
	macMinVersion = "10.14"
)

func main() {
	var (
		arch      string
		system    string
		directory string
		source    string
		ldFlags   string
		gcFlags   string
		buildName string
		output    string
		goBin     string

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
	flag.StringVar(&gcFlags, "gcflags", "", "specify any gcflags to pass to go build")
	flag.StringVar(&buildName, "buildName", "", "use GOOS_ARCH to specify target platform")
	flag.StringVar(&goBin, "goBinary", "go", "specify path to go binary")
	flag.StringVar(&output, "output", "", "specify the name of executable")
	flag.Parse()

	if buildName != "" {
		parts := strings.Split(buildName, "_")

		if len(parts) != 2 {
			fmt.Fprint(os.Stderr, "buildName must be in GOOS_GOARCH format")
			os.Exit(1)
		}

		system = parts[0]
		arch = parts[1]
	} else {
		buildName = fmt.Sprintf("%s_%s", system, arch)
	}

	cmd := exec.Command(goBin, "build")

	// -trimpath removes absolute file system paths from the final compiled
	// binary.
	cmd.Args = append(cmd.Args, "-trimpath")

	ldf := fmt.Sprintf("-ldflags=%s", ldFlags)
	ldfQuoted := fmt.Sprintf("-ldflags=\"%s\"", ldFlags)
	cmd.Args = append(cmd.Args, ldf)

	gcf := fmt.Sprintf("-gcflags=%s", gcFlags)
	gcfQuoted := fmt.Sprintf("-gcflags=\"%s\"", gcFlags)
	cmd.Args = append(cmd.Args, gcf)

	cmd.Env = os.Environ()
	if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
		cmd.Env = append(cmd.Env, "TMPDIR="+strings.Replace(tmpdir, `\`, `\\`, -1))
	}
	// Disable cgo so that the compiled binary is statically linked. This is useful for systems lacking
	// libc support. macOS is excluded because it will have libc support and cgo is needed for gopsutil on macOS.
	// Always set it explicitly because the default varies. See https://pkg.go.dev/cmd/cgo#hdr-Using_cgo_with_the_go_command.
	if system == macGOOS {
		cmd.Env = append(cmd.Env, "CGO_ENABLED=1")
		// Specify the minimum OS version the build should target. This is necessary
		// for clang to produce a binary that can run on older versions of macOS.
		// This method for passing build arguments through cgo to clang is suggested
		// at https://github.com/golang/go/issues/18400#issuecomment-270414574.
		cmd.Env = append(cmd.Env, fmt.Sprintf("CGO_LDFLAGS=-mmacosx-version-min=%s", macMinVersion))
		cmd.Env = append(cmd.Env, fmt.Sprintf("CGO_CFLAGS=-mmacosx-version-min=%s", macMinVersion))
		cmd.Args = append(cmd.Args, "-installsuffix=evergreen")
	} else {
		cmd.Env = append(cmd.Env, "CGO_ENABLED=0")
	}

	goos := "GOOS=" + system
	goarch := "GOARCH=" + arch
	cmd.Env = append(cmd.Env, goos, goarch)

	cmd.Args = append(cmd.Args, "-o", output)
	cmd.Args = append(cmd.Args, source)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmdString := strings.Join(cmd.Args, " ")
	cmdString = strings.Replace(cmdString, gcf, gcfQuoted, -1)
	fmt.Println(goos, goarch, strings.Replace(cmdString, ldf, ldfQuoted, -1))
	if err := cmd.Run(); err != nil {
		fmt.Printf("problem building %s: %v\n", output, err)
		os.Exit(1)
	}
}
