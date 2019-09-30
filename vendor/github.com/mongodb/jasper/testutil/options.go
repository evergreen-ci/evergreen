package testutil

import (
	"fmt"
	"runtime"
	"time"

	"github.com/mongodb/jasper/options"
	"github.com/tychoish/bond"
)

func YesCreateOpts(timeout time.Duration) *options.Create {
	return &options.Create{Args: []string{"yes"}, Timeout: timeout}
}

func TrueCreateOpts() *options.Create {
	return &options.Create{
		Args: []string{"true"},
	}
}

func FalseCreateOpts() *options.Create {
	return &options.Create{
		Args: []string{"false"},
	}
}

func SleepCreateOpts(num int) *options.Create {
	return &options.Create{
		Args: []string{"sleep", fmt.Sprint(num)},
	}
}

func ValidMongoDBDownloadOptions() options.MongoDBDownload {
	target := runtime.GOOS
	if target == "darwin" {
		target = "osx"
	}

	edition := "enterprise"
	if target == "linux" {
		edition = "base"
	}

	return options.MongoDBDownload{
		BuildOpts: bond.BuildOptions{
			Target:  target,
			Arch:    bond.MongoDBArch("x86_64"),
			Edition: bond.MongoDBEdition(edition),
			Debug:   false,
		},
		Releases: []string{"4.0-current"},
	}
}
