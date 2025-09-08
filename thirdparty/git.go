package thirdparty

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	gitHubURL = "github.com"
)

// Summary stores summary patch information
type Summary struct {
	Name        string `bson:"filename"`
	Additions   int    `bson:"additions"`
	Deletions   int    `bson:"deletions"`
	Description string `bson:"description,omitempty"`
}

type Commit struct {
	Summaries []Summary `bson:"summaries"`
	Message   string    `bson:"message"`
	Patch     string    `bson:"patch"`
}

// GitApplyNumstat attempts to apply a given patch; it returns the patch's bytes
// if it is successful
func GitApplyNumstat(patch string) (*bytes.Buffer, error) {
	handle, err := os.CreateTemp("", utility.RandomString())
	if err != nil {
		return nil, errors.Wrapf(err, "creating local patch file")
	}
	defer func() {
		grip.Error(handle.Close())
		grip.Error(os.Remove(handle.Name()))
	}()
	// convert the patch to bytes
	buf := []byte(patch)
	buffer := bytes.NewBuffer(buf)
	for {
		// read a chunk
		n, err := buffer.Read(buf)
		if err != nil && err != io.EOF {
			return nil, errors.Wrapf(err, "reading supplied patch file")
		}
		if n == 0 {
			break
		}
		// write a chunk
		if _, err := handle.Write(buf[:n]); err != nil {
			return nil, errors.Wrapf(err, "writing supplied patch file")
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
		return nil, errors.Wrapf(err, "starting 'git apply --numstat': 424 - %v",
			summaryBuffer.String())
	}

	// this should never happen if patch is initially validated
	if err := cmd.Wait(); err != nil {
		return nil, errors.Wrapf(err, "running 'git apply --numstat': 562 - %v",
			summaryBuffer.String())
	}
	return &summaryBuffer, nil
}

// ParseGitSummary takes in a buffer of data and parses it into a slice of
// git summaries. It returns an error if it is unable to parse the data
func ParseGitSummary(gitOutput fmt.Stringer) (summaries []Summary, err error) {
	// separate stats per file
	fileStats := strings.Split(gitOutput.String(), "\n")

	var additions, deletions int

	for _, fileDetails := range fileStats {
		details := strings.SplitN(fileDetails, "\t", 3)
		// we expect to get the number of additions,
		// the number of deletions, and the filename
		if len(details) != 3 {
			grip.Debug(message.Fields{
				"message": "file stat details has unexpected length",
				"details": details,
				"length":  len(details),
			})
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

		summary := Summary{
			Name:      details[2],
			Additions: additions,
			Deletions: deletions,
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func GetPatchSummaries(patchContent string) ([]Summary, error) {
	summaries := []Summary{}
	if patchContent != "" {
		gitOutput, err := GitApplyNumstat(patchContent)
		if err != nil {
			return nil, errors.Wrap(err, "validating patch")
		}
		if gitOutput == nil {
			return nil, errors.New("validating patch: `git apply --numstat` returned empty")
		}

		summaries, err = ParseGitSummary(gitOutput)
		if err != nil {
			return nil, errors.Wrap(err, "parsing git summary")
		}
	}
	return summaries, nil
}

func ParseGitUrl(url string) (string, string, error) {
	var owner, repo string
	httpsPrefix := "https://"
	if strings.HasPrefix(url, httpsPrefix) {
		url = strings.TrimPrefix(url, httpsPrefix)
		split := strings.Split(url, "/")
		if len(split) != 3 {
			// this covers the case of a GitHub API URL of the form
			// https://api.github.com/repos/<owner>/<repo>
			if split[0] == "api.github.com" && len(split) > 3 {
				return split[2], split[3], nil
			}
			return owner, repo, errors.Errorf("Invalid Git URL format '%s'", url)
		}
		owner = split[1]
		splitRepo := strings.Split(split[2], ".")
		if len(splitRepo) != 2 {
			return owner, repo, errors.Errorf("Invalid Git URL format '%s'", url)
		}
		repo = splitRepo[0]
	} else {
		splitModule := strings.Split(url, ":")
		if len(splitModule) != 2 {
			return owner, repo, errors.Errorf("Invalid Git URL format '%s'", url)
		}
		splitOwner := strings.Split(splitModule[1], "/")
		if len(splitOwner) != 2 {
			return owner, repo, errors.Errorf("Invalid Git URL format '%s'", url)
		}
		owner = splitOwner[0]
		splitRepo := strings.Split(splitOwner[1], ".")
		if len(splitRepo) != 2 {
			return owner, repo, errors.Errorf("Invalid Git URL format '%s'", url)
		}
		repo = splitRepo[0]
	}

	if len(owner) == 0 || len(repo) == 0 {
		return "", "", errors.Errorf("Couldn't parse owner and repo from %s", url)
	}
	return owner, repo, nil
}

func FormGitURL(owner, repo, token string) string {
	if token != "" {
		return fmt.Sprintf("https://%s:x-oauth-basic@%s/%s/%s.git", token, gitHubURL, owner, repo)
	}

	return fmt.Sprintf("https://%s/%s/%s.git", gitHubURL, owner, repo)
}

func FormGitURLForApp(owner, repo, token string) string {
	if token != "" {
		return fmt.Sprintf("https://x-access-token:%s@%s/%s/%s.git", token, gitHubURL, owner, repo)
	}

	return fmt.Sprintf("https://%s/%s/%s.git", gitHubURL, owner, repo)
}

// ParseGitVersion parses the git version number from the version string and returns a boolean indicating if it's an Apple Git version.
func ParseGitVersion(version string) (string, error) {
	appleGitRegex := `(?: \(Apple Git-[\w\.]+\))?$`
	matches := regexp.MustCompile(`^git version ` +
		// capture the version major.minor(.patch(.build(.etc...)))
		`(\w+(?:\.\w+)+)` +
		// match and capture Apple git's addition to the version string
		appleGitRegex,
	).FindStringSubmatch(version)
	if len(matches) != 2 {
		return "", errors.Errorf("could not parse git version number from version string '%s'", version)
	}

	return matches[1], nil
}
