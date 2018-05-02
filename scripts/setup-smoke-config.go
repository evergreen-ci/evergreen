package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

func main() {
	var (
		githubToken string

		clientsDir      = "clients"
		testDataDir     = "testdata"
		smokeConfigFile = filepath.Join(testDataDir, "smoke_config.yml")
	)

	flag.StringVar(&githubToken, "githubToken", "", "Github token")
	flag.Parse()

	err := os.MkdirAll(clientsDir, 0777)
	if err != nil {
		fmt.Printf(errors.Wrap(err, "unable to create clients directory").Error())
		return
	}

	f, err := os.OpenFile(smokeConfigFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf(errors.Wrap(err, "unable to open smoke config").Error())
		return
	}
	data := fmt.Sprintf(`
log_path: "STDOUT"
credentials: {
  github: "%s"
}`, githubToken)
	_, err = f.Write([]byte(data))
	if err != nil {
		fmt.Printf(errors.Wrap(err, "unable to write smoke config").Error())
		return
	}
	err = f.Close()
	if err != nil {
		fmt.Printf(errors.Wrap(err, "unable to close smoke config").Error())
		return
	}
}
