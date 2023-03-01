package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	getClientSubcommand  = "get-client"
	signSubcommand       = "sign"
	notaryClientFilename = "macnotary"
)

func zipFile(inputFilePath, destinationPath string) error {
	outputFile, err := os.Create(destinationPath)
	if err != nil {
		return errors.Wrap(err, "creating zip file")
	}
	defer outputFile.Close()

	zipWriter := zip.NewWriter(outputFile)
	defer zipWriter.Close()

	inputFile, err := os.Open(inputFilePath)
	if err != nil {
		return errors.Wrapf(err, "opening input file '%s'", inputFilePath)
	}
	defer inputFile.Close()

	fileInfo, err := inputFile.Stat()
	if err != nil {
		return errors.Wrap(err, "getting info for input file")
	}

	header, err := zip.FileInfoHeader(fileInfo)
	if err != nil {
		return errors.Wrap(err, "making header from file info")
	}
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return errors.Wrap(err, "writing header to zip file")
	}
	_, err = io.Copy(writer, inputFile)
	return errors.Wrap(err, "copying input file into zip file")
}

func extractToFile(file *zip.File, destination string) error {
	fileReader, err := file.Open()
	if err != nil {
		return errors.Wrap(err, "opening zip file")
	}
	defer fileReader.Close()

	contents, err := io.ReadAll(fileReader)
	if err != nil {
		return errors.Wrap(err, "reading zip file")
	}

	return errors.Wrap(os.WriteFile(destination, contents, file.Mode()), "writing zip contents to file")
}

func extractFileFromZip(zipBytes []byte, targetFile, destination string) error {
	reader := bytes.NewReader(zipBytes)
	zipReader, err := zip.NewReader(reader, reader.Size())
	if err != nil {
		return errors.Wrap(err, "getting zip reader")
	}

	targetFound := false
	for _, file := range zipReader.File {
		if path.Base(file.Name) != targetFile {
			continue
		}
		if file.FileInfo().IsDir() {
			continue
		}
		targetFound = true
		if err = extractToFile(file, destination); err != nil {
			return errors.Wrap(err, "extracting zip file")
		}
	}

	if !targetFound {
		return errors.Errorf("target file not in the archive")
	}

	return nil
}

func downloadZip(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "making get request")
	}
	req.Header.Add("Content-Type", "application/zip")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "making request")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("got '%d' status code", resp.StatusCode)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}

	return body, nil
}

func signWithNotaryClient(ctx context.Context, zipToSignPath, destinationPath string, opts signOpts) error {
	args := []string{
		"--url",
		opts.notaryServerURL,
		"--file",
		zipToSignPath,
		"--mode",
		"notarizeAndSign",
		"--bundleId",
		opts.bundleID,
		"--out-path",
		destinationPath,
	}
	if opts.notaryKey != "" {
		args = append(args, "--key-id", opts.notaryKey)
	}
	if opts.notarySecret != "" {
		args = append(args, "--secret", opts.notarySecret)
	}

	cmd := exec.CommandContext(ctx, opts.notaryClientPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return errors.Wrap(cmd.Run(), "running notary client")
}

func downloadClient(ctx context.Context, opts fetchClientOpts) error {
	clientZipBytes, err := downloadZip(ctx, opts.notaryClientDownloadURL)
	if err != nil {
		return errors.Wrap(err, "downloading client")
	}

	if err = extractFileFromZip(clientZipBytes, notaryClientFilename, opts.destination); err != nil {
		return errors.Wrap(err, "unzipping client")
	}

	return os.Chmod(opts.destination, 0755)
}

func signExecutable(ctx context.Context, opts signOpts) error {
	tempDir, err := os.MkdirTemp("", "evergreen")
	if err != nil {
		return errors.Wrap(err, "creating temp dir")
	}
	defer os.RemoveAll(tempDir)

	toSignPath := path.Join(tempDir, "evergreen.zip")
	if err := zipFile(opts.executablePath, toSignPath); err != nil {
		return errors.Wrapf(err, "compressing '%s'", opts.executablePath)
	}

	signedZipPath := path.Join(tempDir, "evergreen_signed.zip")
	if err = signWithNotaryClient(ctx, toSignPath, signedZipPath, opts); err != nil {
		fmt.Fprintf(os.Stderr, "code signing failed: %s", err.Error())
		return nil
	}

	signedZip, err := os.ReadFile(signedZipPath)
	if err != nil {
		return errors.Wrap(err, "reading signed zip")
	}

	return extractFileFromZip(signedZip, path.Base(opts.executablePath), opts.outputPath)
}

type signOpts struct {
	notaryClientPath string
	notaryServerURL  string
	bundleID         string
	executablePath   string
	outputPath       string
	notaryKey        string
	notarySecret     string
}

type fetchClientOpts struct {
	notaryClientDownloadURL string
	destination             string
}

func main() {
	var fetchOpts fetchClientOpts
	var signOpts signOpts
	ctx := context.Background()

	app := &cli.App{
		Commands: []cli.Command{
			{
				Name:  getClientSubcommand,
				Usage: "fetch the notary client",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "destination",
						Usage:       "`PATH` to save the client",
						Destination: &fetchOpts.destination,
						Value:       path.Join(".", notaryClientFilename),
					},
					&cli.StringFlag{
						Name:        "download-url",
						Usage:       "`URL` to GET the client from",
						Destination: &fetchOpts.notaryClientDownloadURL,
						Required:    true,
					},
				},
				Action: func(c *cli.Context) error {
					return downloadClient(ctx, fetchOpts)
				},
			},
			{
				Name:  signSubcommand,
				Usage: "sign an executable",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "client",
						Usage:       "`PATH` to the notary client",
						Destination: &signOpts.notaryClientPath,
						Required:    true,
					},
					&cli.StringFlag{
						Name:        "server-url",
						Usage:       "`PATH` to the notary server",
						Destination: &signOpts.notaryServerURL,
						Required:    true,
					},
					&cli.StringFlag{
						Name:        "bundle-id",
						Usage:       "`BUNDLE-ID` to sign with",
						Destination: &signOpts.bundleID,
						Required:    true,
					},
					&cli.StringFlag{
						Name:        "executable",
						Usage:       "`PATH` to the executable to sign",
						Destination: &signOpts.executablePath,
						Required:    true,
					},
					&cli.StringFlag{
						Name:        "output",
						Usage:       "`PATH` to output the signed executable. If omitted the input executable will be overwritten",
						Destination: &signOpts.outputPath,
					},
					&cli.StringFlag{
						Name:        "notary-key",
						Usage:       "`KEY` to authenticate with the notary server. If omitted the notary client will check the value of MACOS_NOTARY_KEY in the environment",
						Destination: &signOpts.notaryKey,
					},
					&cli.StringFlag{
						Name:        "notary-secret",
						Usage:       "`SECRET` to authenticate with the notary server. If omitted the notary client will check the value of MACOS_NOTARY_SECRET in the environment",
						Destination: &signOpts.notarySecret,
					},
				},
				Action: func(c *cli.Context) error {
					if signOpts.outputPath == "" {
						signOpts.outputPath = signOpts.executablePath
					}
					return signExecutable(ctx, signOpts)
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
