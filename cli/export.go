package cli

import (
	"context"
	"io"
	"os"

	"github.com/pkg/errors"
)

var (
	GranularityDays    = "day"
	GranularityMinutes = "minute"
	GranularitySeconds = "second"
	GranularityHours   = "hour"

	HostUtilizationStat     = "host"
	AverageScheduledToStart = "avg"
	OptimalMakespanStat     = "makespan"
)

// ExportCommand is used to export statistics
type ExportCommand struct {
	GlobalOpts  *Options `no-flag:"true"`
	JSON        bool     `long:"json" description:"set the format to export to json"`
	Granularity string   `long:"granularity" description:"set the granularity, default hour, options are 'second', 'minute', 'hour'"`
	Days        int      `long:"days" description:"set the number of days, default 1, max of 30 days back"`
	StatsType   string   `long:"stat" description:"include the type of stats - 'host' for host utilization,'avg' for average scheduled to start times, 'makespan' for makespan ratios" required:"true"`
	DistroId    string   `long:"distro" description:"distro id - required for average scheduled to start times"`
	Number      int      `long:"number" description:"set the number of revisions (for getting build makespan), default 100"`
	Filepath    string   `long:"filepath" description:"path to directory where csv file is to be saved"`
}

func (ec *ExportCommand) Execute(_ []string) error {
	if ec.Filepath == "" {
		return errors.New("must specify a filepath to write output")
	}

	if ec.StatsType == "" {
		return errors.New("Must specify a stats type, host")
	}

	ctx := context.Background()
	_, rc, _, err := getAPIClients(ctx, ec.GlobalOpts)
	if err != nil {
		return err
	}

	// default granularity to an hour
	if ec.Granularity == "" {
		ec.Granularity = GranularityHours
	}

	// default days to 1
	if ec.Days == 0 {
		ec.Days = 1
	}

	if ec.Number == 0 {
		ec.Number = 100
	}

	isCSV := !ec.JSON

	var body io.ReadCloser
	var granSeconds int

	switch ec.StatsType {
	case HostUtilizationStat:
		granSeconds, err = convertGranularityToSeconds(ec.Granularity)
		if err != nil {
			return err
		}
		body, err = rc.GetHostUtilizationStats(granSeconds, ec.Days, isCSV)
		if err != nil {
			return err
		}
	case AverageScheduledToStart:
		if ec.DistroId == "" {
			return errors.New("cannot have empty distro id")
		}
		granSeconds, err = convertGranularityToSeconds(ec.Granularity)
		if err != nil {
			return err
		}
		body, err = rc.GetAverageSchedulerStats(granSeconds, ec.Days, ec.DistroId, isCSV)
		if err != nil {
			return err
		}
	case OptimalMakespanStat:
		body, err = rc.GetOptimalMakespans(ec.Number, isCSV)
		if err != nil {
			return err
		}

	default:
		return errors.Errorf("%v is not a valid stats type. The current valid types include, host, avg, and makespan", ec.StatsType)

	}

	return WriteToFile(body, ec.Filepath)
}

// convertGranularityToSeconds takes in a string granularity and returns its
func convertGranularityToSeconds(granString string) (int, error) {
	switch granString {
	case GranularityDays:
		return 24 * 60 * 60, nil
	case GranularityHours:
		return 60 * 60, nil
	case GranularityMinutes:
		return 60, nil
	case GranularitySeconds:
		return 1, nil
	default:
		return 0, errors.Errorf("not a valid granularity, %v", granString)
	}
}

// WriteToFile takes in a body and filepath and writes out the data in the body
func WriteToFile(body io.ReadCloser, filepath string) error {
	defer body.Close()
	if filepath == "" {
		return errors.New("cannot write output to an unspecified ")
	}

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, body)
	return err
}
