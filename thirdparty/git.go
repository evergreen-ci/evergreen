package thirdparty

import (
	"bytes"
	"io"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// GitApplyNumstat attempts to apply a given patch; it returns the patch's bytes
// if it is successful
func GitApplyNumstat(patch string) (*bytes.Buffer, error) {
	handle, err := ioutil.TempFile("", util.RandomString())
	if err != nil {
		return nil, errors.New("Unable to create local patch file")
	}
	// convert the patch to bytes
	buf := []byte(patch)
	buffer := bytes.NewBuffer(buf)
	for {
		// read a chunk
		n, err := buffer.Read(buf)
		if err != nil && err != io.EOF {
			return nil, errors.New("Unable to read supplied patch file")
		}
		if n == 0 {
			break
		}
		// write a chunk
		if _, err := handle.Write(buf[:n]); err != nil {
			return nil, errors.New("Unable to read supplied patch file")
		}
	}

	// pseudo-validate the patch set by attempting to get a summary
	var summaryBuffer bytes.Buffer
	cmd := exec.Command("git", "apply", "--numstat", handle.Name())
	cmd.Stdout = &summaryBuffer
	cmd.Stderr = &summaryBuffer
	cmd.Dir = filepath.Dir(handle.Name())

	// this should never happen if patch is initially validated
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrapf(err, "Error validating patch: 424 - %v",
			summaryBuffer.String())
	}

	// this should never happen if patch is initially validated
	if err := cmd.Wait(); err != nil {
		return nil, errors.Wrapf(err, "Error waiting on patch: 562 - %v",
			summaryBuffer.String())
	}
	return &summaryBuffer, nil
}

// ParseGitSummary takes in a buffer of data and parses it into a slice of
// git summaries. It returns an error if it is unable to parse the data
func ParseGitSummary(gitOutput *bytes.Buffer) (summaries []patch.Summary, err error) {
	// separate stats per file
	fileStats := strings.Split(gitOutput.String(), "\n")

	var additions, deletions int

	for _, fileDetails := range fileStats {
		details := strings.SplitN(fileDetails, "\t", 3)
		// we expect to get the number of additions,
		// the number of deletions, and the filename
		if len(details) != 3 {
			grip.Errorf("File stat details for '%v' has length '%v'",
				details, len(details))
			continue
		}

		additions, err = strconv.Atoi(details[0])
		if err != nil {
			if details[0] == "-" {
				grip.Warningf("Line addition count for %v is '%v' assuming "+
					"binary data diff, using 0", details[2], details[0])
				additions = 0
			} else {
				return nil, errors.Wrap(err, "Error getting patch additions summary")
			}
		}

		deletions, err = strconv.Atoi(details[1])
		if err != nil {
			if details[1] == "-" {
				grip.Warningf("Line deletion count for %v is '%v' assuming "+
					"binary data diff, using 0", details[2], details[1])
				deletions = 0
			} else {
				return nil, errors.Wrap(err, "Error getting patch deletions summary")
			}
		}

		summary := patch.Summary{
			Name:      details[2],
			Additions: additions,
			Deletions: deletions,
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}
