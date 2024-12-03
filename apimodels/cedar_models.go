package apimodels

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/buildlogger"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

type CedarConfig struct {
	BaseURL      string `json:"base_url"`
	GRPCBaseURL  string `json:"grpc_base_url"`
	RPCPort      string `json:"rpc_port"`
	Username     string `json:"username"`
	APIKey       string `json:"api_key,omitempty"`
	Insecure     bool   `json:"insecure"`
	SPSURL       string `json:"sps_url"`
	SPSKanopyURL string `json:"sps_kanopy_url"`
}

// GetBuildloggerLogsOptions represents the arguments passed into the
// GetBuildloggerLogs function.
type GetBuildloggerLogsOptions struct {
	BaseURL   string   `json:"_"`
	TaskID    string   `json:"-"`
	Execution *int     `json:"-"`
	TestName  string   `json:"-"`
	Tags      []string `json:"-"`
	Start     int64    `json:"-"`
	End       int64    `json:"-"`
	Limit     int      `json:"-"`
	Tail      int      `json:"-"`
}

// GetBuildloggerLogs makes request to Cedar for a specifc log and returns a
// log iterator.
// TODO (DEVPROD-1681): Remove this once Cedar logs have TTL'ed.
func GetBuildloggerLogs(ctx context.Context, opts GetBuildloggerLogsOptions) (log.LogIterator, error) {
	usr := gimlet.GetUser(ctx)
	if usr == nil {
		return nil, errors.New("error getting user from context")
	}

	var start, end time.Time
	if opts.Start > 0 {
		start = time.Unix(0, opts.Start).UTC()
	}
	if opts.End > 0 {
		end = time.Unix(0, opts.End).UTC()
	}

	getOpts := buildlogger.GetOptions{
		Cedar: timber.GetOptions{
			BaseURL:  fmt.Sprintf("https://%s", opts.BaseURL),
			UserKey:  usr.GetAPIKey(),
			UserName: usr.Username(),
		},
		TaskID:        opts.TaskID,
		Execution:     opts.Execution,
		TestName:      opts.TestName,
		Tags:          opts.Tags,
		PrintTime:     true,
		PrintPriority: true,
		Start:         start,
		End:           end,
		Limit:         opts.Limit,
		Tail:          opts.Tail,
	}
	r, err := buildlogger.Get(ctx, getOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "getting logs from Cedar Buildlogger")
	}

	return newBuildloggerIterator(r), nil
}

// TODO (DEVPROD-1681): Remove this once Cedar logs have TTL'ed.
type buildloggerIterator struct {
	readCloser io.ReadCloser
	reader     *bufio.Reader
	item       log.LogLine
	catcher    grip.Catcher
	exhausted  bool
	closed     bool
}

func newBuildloggerIterator(r io.ReadCloser) *buildloggerIterator {
	return &buildloggerIterator{
		readCloser: r,
		reader:     bufio.NewReader(r),
		catcher:    grip.NewBasicCatcher(),
	}
}

func (it *buildloggerIterator) Next() bool {
	if it.closed || it.exhausted {
		return false
	}

	line, err := it.reader.ReadString('\n')
	if err != nil {
		it.exhausted = err == io.EOF
		it.catcher.AddWhen(err != io.EOF, errors.Wrap(err, "reading log lines"))
		return false
	}

	// Each log line is expected to have the format:
	//     [P:3%d] [2006/01/02 15:04:05.000] %s
	// Fail if we cannot parse the first 34 characters to avoid panicking.
	if len(line) < 34 {
		it.catcher.Errorf("malformed line '%s'", line)
		return false
	}

	// Parse prefixed priority with format "[P:3%d] %s".
	priority, err := strconv.Atoi(strings.TrimSpace(line[3:6]))
	if err != nil {
		it.catcher.Wrap(err, "parsing log line priority")
		return false
	}
	line = line[8:]

	// Parse prefixed timestamp with format "[2006/01/02 15:04:05.000] %s".
	ts, err := time.Parse("2006/01/02 15:04:05.000", line[1:24])
	if err != nil {
		it.catcher.Wrap(err, "parsing log line timestamp")
		return false
	}
	line = line[26:]

	it.item = log.LogLine{
		Priority:  level.Priority(priority),
		Timestamp: ts.UnixNano(),
		Data:      strings.TrimSuffix(line, "\n"),
	}

	return true
}

func (it *buildloggerIterator) Item() log.LogLine { return it.item }

func (it *buildloggerIterator) Exhausted() bool { return it.exhausted }

func (it *buildloggerIterator) Err() error { return it.catcher.Resolve() }

func (it *buildloggerIterator) Close() error {
	if it.closed {
		return nil
	}
	it.closed = true

	return it.readCloser.Close()
}
