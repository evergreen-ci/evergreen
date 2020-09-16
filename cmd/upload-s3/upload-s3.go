package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/evergreen-ci/pail"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func main() {
	app := makeApp()
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
	}
}

func makeApp() *cli.App {
	app := cli.NewApp()
	app.Name = "upload-s3"
	app.Usage = "Upload file(s) to S3."
	app.Flags = []cli.Flag{
		cli.StringSliceFlag{
			Name:  "file,f",
			Usage: "The local file(s) to upload to S3.",
		},
		cli.StringFlag{
			Name:  "prefix, p",
			Usage: "The path prefix in S3 where the file(s) should be uploaded.",
		},
		cli.StringFlag{
			Name:  "bucket, b",
			Usage: "The name of the S3 bucket.",
		},
		cli.StringFlag{
			Name:   "key, k",
			Usage:  "The AWS key for uploading to S3. If this is not given, the AWS credentials file must be given.",
			EnvVar: "AWS_UPLOAD_KEY",
		},
		cli.StringFlag{
			Name:   "secret, s",
			Usage:  "The AWS secret for uploading to S3. If this is not given, the AWS credentials file must be given.",
			EnvVar: "AWS_UPLOAD_SECRET",
		},
		cli.StringFlag{
			Name:  "credentials_file, c",
			Usage: "The file containing the AWS credentials. If this is not given, the AWS key and secret must both be given.",
		},
	}
	app.Before = func(c *cli.Context) error {
		catcher := grip.NewBasicCatcher()
		catcher.NewWhen(len(c.StringSlice("file")) == 0, "missing file")
		catcher.NewWhen(c.String("bucket") == "", "missing bucket")
		if c.String("credentials_file") == "" && (c.String("key") == "" || c.String("secret") == "") {
			catcher.New("must specify either credentials file or AWS key/secret")
		}
		if c.String("credentials_file") != "" && (c.String("key") != "" || c.String("secret") == "") {
			catcher.New("cannot specify both credentials file and AWS key/secret")
		}
		return catcher.Resolve()
	}
	app.Action = uploadToS3
	return app
}

func uploadToS3(c *cli.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bucketName := c.String("bucket")

	b, err := pail.NewS3Bucket(pail.S3Options{
		Name:                      bucketName,
		Credentials:               pail.CreateAWSCredentials(c.String("key"), c.String("secret"), ""),
		SharedCredentialsFilepath: c.String("credentials_file"),
		Region:                    "us-east-1",
		Permissions:               pail.S3PermissionsPublicRead,
	})
	if err != nil {
		return errors.Wrap(err, "initializing bucket")
	}

	catcher := grip.NewBasicCatcher()
	for _, file := range c.StringSlice("file") {
		remotePath := path.Join(c.String("prefix"), filepath.Base(file))
		fmt.Printf("Uploading '%s' to '%s'\n", file, path.Join(bucketName, remotePath))
		if err := b.Upload(ctx, remotePath, file); err != nil {
			catcher.Wrapf(err, "uploading file '%s'", file)
			continue
		}
		url := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, remotePath)
		fmt.Printf("Object URL: %s\n", url)
	}

	return catcher.Resolve()
}
