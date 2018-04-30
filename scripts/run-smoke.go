package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
)

func main() {
	var (
		githubToken string
		awsKey      string
		awsSecret   string

		clientsDir      = "clients"
		testDataDir     = "testdata"
		smokeConfigFile = filepath.Join(testDataDir, "smoke_config.yml")
	)

	flag.StringVar(&githubToken, "githubToken", "", "Github token")
	flag.StringVar(&awsKey, "awsKey", "", "AWS key")
	flag.StringVar(&awsSecret, "awsSecret", "", "AWS secret")
	flag.Parse()

	err := os.MkdirAll(clientsDir, 0777)
	if err != nil {
		fmt.Printf(errors.Wrap(err, "unable to create clients directory").Error())
		return
	}

	err = os.MkdirAll(testDataDir, 0777)
	if err != nil {
		fmt.Printf(errors.Wrap(err, "unable to create testdata directory").Error())
		return
	}
	data := fmt.Sprintf(`log_path:"STDOUT"
    credentials: {
    github: "%s",
  }`, githubToken)
	err = ioutil.WriteFile(smokeConfigFile, []byte(data), 0777)
	if err != nil {
		fmt.Printf(errors.Wrap(err, "unable to create create smoke config").Error())
		return
	}

	cmd := exec.Command("bin/set-project-var", "-dbName", "mci_smoke", "-key", "aws_key", "-value", awsKey)
	if err = cmd.Run(); err != nil {
		fmt.Printf(errors.Wrap(err, "unable to set aws_key in project").Error())
		return
	}

	cmd = exec.Command("bin/set-project-var", "-dbName", "mci_smoke", "-key", "aws_secret", "-value", awsSecret)
	if err = cmd.Run(); err != nil {
		fmt.Printf(errors.Wrap(err, "unable to set aws_secret in project").Error())
		return
	}

	cmd = exec.Command("git", "rev-parse", "HEAD")
	var rev bytes.Buffer
	cmd.Stdout = &rev
	if err = cmd.Run(); err != nil {
		fmt.Printf(errors.Wrap(err, "unable to determine revision").Error())
		return
	}

	cmd = exec.Command("bin/set-var", "-dbName", "mci_smoke", "-collection", "hosts",
		"-id", "localhost", "-key", "agent_revision", "-value", rev.String())
	if err = cmd.Run(); err != nil {
		fmt.Printf(errors.Wrap(err, "unable to set project variables").Error())
	}
}
