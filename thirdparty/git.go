package thirdparty

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
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

func GetPatchSummariesFromMboxPatch(mboxPatch string) ([]Summary, []string, error) {
	commits, err := getCommitsFromMboxPatch(mboxPatch)
	if err != nil {
		return nil, nil, errors.Wrap(err, "can't get commits from patch")
	}

	summaries := []Summary{}
	commitMessages := []string{}
	for _, commit := range commits {
		for i := range commit.Summaries {
			commit.Summaries[i].Description = commit.Message
		}
		summaries = append(summaries, commit.Summaries...)
		commitMessages = append(commitMessages, commit.Message)
	}

	return summaries, commitMessages, nil
}

func GetDiffsFromMboxPatch(patchContent string) (string, error) {
	commits, err := getCommitsFromMboxPatch(patchContent)
	if err != nil {
		return "", errors.Wrap(err, "problem getting commits from patch")
	}
	diffs := make([]string, 0, len(commits))
	for _, commit := range commits {
		diffs = append(diffs, commit.Patch)
	}

	return strings.Join(diffs, "\n"), nil
}

// getCommitsFromMboxPatch returns commit information from an mbox patch
func getCommitsFromMboxPatch(mboxPatch string) ([]Commit, error) {
	tmpDir, err := os.MkdirTemp("", "patch_summaries_by_commit")
	if err != nil {
		return nil, errors.Wrap(err, "problem creating temporary directory")
	}
	defer os.RemoveAll(tmpDir)

	mailSplitCommand := exec.Command("git", "mailsplit", "--keep-cr", fmt.Sprintf("-o%s", tmpDir))
	mailSplitCommand.Stdin = strings.NewReader(mboxPatch)
	if err = mailSplitCommand.Run(); err != nil {
		return nil, errors.Wrap(err, "problem splitting patch content")
	}

	commits := []Commit{}
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, errors.Wrap(err, "problem listing split patches")
	}
	for i, file := range files {
		fileReader, err := os.Open(filepath.Join(tmpDir, file.Name()))
		if err != nil {
			return nil, errors.Wrap(err, "can't open individual patch file")
		}

		msgFile := filepath.Join(tmpDir, fmt.Sprintf("%d_message.txt", i))
		patchFile := filepath.Join(tmpDir, fmt.Sprintf("%d_patch.txt", i))
		mailInfoCommand := exec.Command("git", "mailinfo", msgFile, patchFile)
		mailInfoCommand.Stdin = fileReader
		out, err := mailInfoCommand.CombinedOutput()
		if err != nil {
			return nil, errors.Wrap(err, "problem getting mailinfo from patches")
		}
		commitMessage, err := parseCommitMessage(string(out))
		if err != nil {
			return nil, errors.Wrap(err, "problem parsing commit message")
		}

		// If the commit message contains an extended description mailinfo will
		// split off the description from the summary and write it to msgFile.
		// Append an ellipsis to the message to indicate there's more to the message
		// that git am will use when it creates the commit.
		msgFileInfo, err := os.Stat(msgFile)
		if err != nil {
			return nil, errors.Wrap(err, "problem getting message file info")
		}
		if msgFileInfo.Size() > 0 {
			commitMessage = fmt.Sprintf("%s...", commitMessage)
		}

		patchBytes, err := os.ReadFile(patchFile)
		if err != nil {
			return nil, errors.Wrap(err, "problem reading patch file")
		}
		patchSummaries, err := GetPatchSummaries(string(patchBytes))
		if err != nil {
			return nil, errors.Wrap(err, "problem getting commit summaries for patch")
		}

		commits = append(commits, Commit{
			Summaries: patchSummaries,
			Message:   commitMessage,
			Patch:     string(patchBytes),
		})
	}

	return commits, nil
}

func parseCommitMessage(commitHeaders string) (string, error) {
	lines := strings.Split(commitHeaders, "\n")
	subjectPrefix := "Subject: "
	for _, line := range lines {
		if strings.HasPrefix(line, subjectPrefix) {
			return strings.TrimPrefix(line, subjectPrefix), nil
		}
	}
	return "", errors.Errorf("subject line not found in headers '%s'", commitHeaders)
}

func ParseGitUrl(url string) (string, string, error) {
	var owner, repo string
	httpsPrefix := "https://"
	if strings.HasPrefix(url, httpsPrefix) {
		url = strings.TrimPrefix(url, httpsPrefix)
		split := strings.Split(url, "/")
		if len(split) != 3 {
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

func FormGitURL(host, owner, repo, token string) string {
	if token != "" {
		return fmt.Sprintf("https://%s:x-oauth-basic@%s/%s/%s.git", token, host, owner, repo)
	}

	return fmt.Sprintf("git@%s:%s/%s.git", host, owner, repo)
}

func FormGitURLForApp(host, owner, repo, token string) string {
	if token != "" {
		return fmt.Sprintf("https://x-access-token:%s@%s/%s/%s.git", token, host, owner, repo)
	}

	return fmt.Sprintf("git@%s:%s/%s.git", host, owner, repo)
}
