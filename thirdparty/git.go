package thirdparty

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

// GetGitHubFileFromGit retrieves a single file's contents from GitHub using
// git. Ref must be a commit hash or branch.
func GetGitHubFileFromGit(ctx context.Context, owner, repo, ref, file string) ([]byte, error) {
	ctx, span := tracer.Start(ctx, "GetGitHubFileFromGit")
	defer span.End()

	dir, err := GitCloneMinimal(ctx, owner, repo, ref)
	if err != nil {
		return nil, errors.Wrap(err, "git cloning repository")
	}
	defer func() {
		grip.Warning(message.WrapError(os.RemoveAll(dir), message.Fields{
			"message": "could not clean up git clone directory",
			"owner":   owner,
			"repo":    repo,
			"ref":     ref,
			"file":    file,
			"ticket":  "DEVPROD-26143",
		}))
	}()

	fileContent, err := gitRestoreFile(ctx, owner, repo, ref, dir, file)
	return fileContent, errors.Wrap(err, "restoring git file")
}

const gitOperationTimeout = 15 * time.Second

// GitCloneMinimal performs a minimal git clone of a repository using the GitHub
// app. The minimal clone contains only git metadata for the one revision and
// has no file content. Callers are expected to clean up the returned git
// directory when it is no longer needed.
func GitCloneMinimal(ctx context.Context, owner, repo, revision string) (string, error) {
	ctx, span := tracer.Start(ctx, "gitCloneMinimal", trace.WithAttributes(
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, revision),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return "", errors.Wrap(err, "creating GitHub app installation token")
	}

	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("git-clone-%s-%s-", owner, repo))
	if err != nil {
		return "", errors.Wrap(err, "creating temp dir for git clone")
	}

	repoURL := FormGitURLForApp(owner, repo, token)

	// Limit how long this can clone to prevent this from running too long. This
	// is an experimental feature and should not meaningfully impact performance
	// while it's being tested out. Realistically, if it took more than this
	// long to do a minimal clone, it would be too slow to be usable.
	ctx, cancel := context.WithTimeout(ctx, gitOperationTimeout)
	defer cancel()

	// Clone the repository with the bare minimum metadata for just the one
	// commit. Don't fetch any actual file blobs yet.
	cmd := exec.CommandContext(ctx, "git", "clone",
		fmt.Sprintf("--revision=%s", revision),
		// Shallow clone: only fetch the one commit rather than the full commit
		// history.
		"--depth=1",
		// Partial clone: don't fetch any file blobs initially (so the repo
		// contains no actual file contents).
		"--filter=blob:none",
		// Don't populate the working directory with any of the files initially.
		"--no-checkout",
		// Don't fetch tags as they're unnecessary extra data.
		"--no-tags",
		repoURL,
		tmpDir,
	)
	var (
		stdout strings.Builder
		stderr strings.Builder
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "minimal git clone failed",
			"ticket":   "DEVPROD-26143",
			"owner":    owner,
			"repo":     repo,
			"revision": revision,
			"stdout":   stdout.String(),
			"stderr":   stderr.String(),
		}))
		catcher := grip.NewBasicCatcher()
		catcher.Wrapf(err, "git cloning repo '%s/%s'", owner, repo)
		catcher.Wrap(os.RemoveAll(tmpDir), "cleaning up temp dir after failed git clone")
		return "", catcher.Resolve()
	}

	return tmpDir, nil
}

// GitCreateWorktree creates a new git worktree in worktreeDir based on dir. It
// does not perform a checkout. Callers are assumed to have already cloned the
// repo into dir and HEAD is assumed to be already pointing to the desired
// revision.
func GitCreateWorktree(ctx context.Context, dir, worktreeDir string) error {
	ctx, span := tracer.Start(ctx, "gitCreateWorktree")
	defer span.End()

	// Limit how long this can spend creating the worktree to prevent this from
	// running too long. This is an experimental feature and should not
	// meaningfully impact performance while it's being tested out.
	// Realistically, if it took more than this long to create a worktree,
	// it would be too slow to be usable.
	ctx, cancel := context.WithTimeout(ctx, gitOperationTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "worktree", "add",
		"--no-checkout",
		"--detach",
		worktreeDir)
	cmd.Dir = dir
	var (
		stdout strings.Builder
		stderr strings.Builder
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":      "git worktree add failed",
			"ticket":       "DEVPROD-26143",
			"worktree_dir": worktreeDir,
			"stdout":       stdout.String(),
			"stderr":       stderr.String(),
		}))
		return errors.Wrapf(err, "creating git worktree '%s'", worktreeDir)
	}

	return nil
}

const gitErrorFileNotFound = "did not match any file(s) known to git"

// gitRestoreFile restores a git file within the given git directory and returns
// its contents. Callers are assumed to have already cloned the repo into dir
// and HEAD is assumed to be already pointing to the desired revision.
func gitRestoreFile(ctx context.Context, owner, repo, revision, dir string, fileName string) ([]byte, error) {
	ctx, span := tracer.Start(ctx, "gitRestoreFile", trace.WithAttributes(
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, revision),
		attribute.String(githubPathAttribute, fileName),
	))
	defer span.End()

	// Validate that the file is within the git directory to prevent attempts to
	// access files outside the git repo. The requested file could be
	// user-provided (e.g. an include file), which is not trusted.
	if err := validateFileIsWithinDirectory(dir, fileName); err != nil {
		return nil, errors.Wrapf(err, "validating file path '%s' is within git repo directory", fileName)
	}

	// Limit how long this can spend restoring the file to prevent this from
	// running too long. This is an experimental feature and should not
	// meaningfully impact performance while it's being tested out.
	// Realistically, if it took more than this long to restore a single file,
	// it would be too slow to be usable.
	ctx, cancel := context.WithTimeout(ctx, gitOperationTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "restore", "--source=HEAD", fileName)
	cmd.Dir = dir
	var (
		stdout strings.Builder
		stderr strings.Builder
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), gitErrorFileNotFound) {
			// To mirror GetGithubFile's behavior, return a FileNotFoundError if
			// the file doesn't exist in the repo at the given revision.
			return nil, FileNotFoundError{filepath: fileName}
		}
		grip.Error(message.WrapError(err, message.Fields{
			"message":   "git restore failed",
			"ticket":    "DEVPROD-26143",
			"owner":     owner,
			"repo":      repo,
			"revision":  revision,
			"stdout":    stdout.String(),
			"stderr":    stderr.String(),
			"file_name": fileName,
		}))
		return nil, errors.Wrapf(err, "restoring file '%s'", fileName)
	}

	// Validate that the restored file is not a symlink to prevent attempts to
	// access a different file in the file system (e.g. a file in the git repo
	// that symlinks to `~/.ssh/id_rsa`).
	if err := validateFileIsNotSymlink(dir, fileName); err != nil {
		return nil, errors.Wrapf(err, "validating file '%s' is not a symlink", fileName)
	}

	contents, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		return nil, errors.Wrapf(err, "reading restored file '%s'", fileName)
	}
	return contents, nil
}

// validateFileIsWithinDirectory ensures that the given file path is
// contained within the specified directory.
func validateFileIsWithinDirectory(dir, file string) error {
	// Normalize the path (e.g. removes redundant separators, resolves ".").
	cleanPath := filepath.Clean(file)

	if filepath.IsAbs(cleanPath) {
		return errors.Errorf("file '%s' must be a relative path, not absolute", file)
	}

	// Reject paths containing ".." (prevent attempts to escape the directory).
	if strings.Contains(cleanPath, "..") {
		return errors.Errorf("file '%s' cannot traverse directories using '..'", file)
	}

	fullFilePath := filepath.Join(dir, cleanPath)

	// Use filepath.Rel to verify fullFilePath is a filepath within dir to rule
	// out any other complex path manipulations.
	relPath, err := filepath.Rel(dir, fullFilePath)
	if err != nil {
		return errors.Wrap(err, "verifying that the file is relative to the directory")
	}

	// If the relative path still contains "..", it may try to escape the
	// directory.
	if strings.Contains(relPath, "..") {
		return errors.Errorf("file '%s' escapes directory using '..'", file)
	}

	return nil
}

// validateFileIsNotSymlink checks that the specified file is not a symlink.
// This protects against symlinks in the git repo pointing outside the
// repository or to unintended files (e.g. a symlink that attempts to read a git
// metadata file).
func validateFileIsNotSymlink(dir, file string) error {
	fullFilePath := filepath.Join(dir, file)
	// Use lstat to get info about the file itself, not its target. In the case
	// of a symlink, it'll get info about the symlink itself and not the file it
	// points to.
	fileInfo, err := os.Lstat(fullFilePath)
	if err != nil {
		return errors.Wrapf(err, "getting stat for file '%s'", file)
	}

	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return errors.Errorf("file '%s' is a symlink, which is disallowed", file)
	}

	return nil
}
