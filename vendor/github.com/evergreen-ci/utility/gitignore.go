package utility

import (
	"os"
	"strings"

	"github.com/pkg/errors"
	ignore "github.com/sabhiram/go-git-ignore"
)

// gitignoreFileMatcher contains the information for building a list of files in the given directory.
// It adds the files to include in the fileNames array and uses the ignorer to determine if a given
// file matches and should be added.
type gitignoreFileMatcher struct {
	ignorer *ignore.GitIgnore
	prefix  string
}

// NewGitignoreFileMatcher returns a FileMatcher that matches the
// expressions rooted at the given prefix. The expressions should be gitignore
// ignore expressions: antyhing that would be matched - and therefore ignored by
// git - is matched.
func NewGitignoreFileMatcher(prefix string, exprs ...string) (FileMatcher, error) {
	ignorer, err := ignore.CompileIgnoreLines(exprs...)
	if err != nil {
		return nil, errors.Wrap(err, "compiling gitignore expressions")
	}
	m := &gitignoreFileMatcher{
		ignorer: ignorer,
		prefix:  prefix,
	}
	return m, nil
}

func (m *gitignoreFileMatcher) Match(file string, info os.FileInfo) bool {
	file = strings.TrimLeft(strings.TrimPrefix(file, m.prefix), string(os.PathSeparator))
	return !info.IsDir() && m.ignorer.MatchesPath(file)
}
