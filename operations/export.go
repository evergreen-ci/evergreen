package operations

import (
	"context"
	"errors"
	"io"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/urfave/cli"
)

const (
	granularityHours   = "hour"
	granularityMinute  = "minute"
	granularitySeconds = "second"
	granularityDays    = "day"
)

func Export() cli.Command {
	const (
		numberFlagName      = "number"
		distroFlagName      = "distro"
		statFlagName        = "stat"
		daysFlagName        = "days"
		granularityFlagName = "granularity"
		jsonFlagName        = "json"

		statHostUtilization   = "host"
		statSecheduledToStart = "avg"
		statOptimalMakespan   = "makespan"
	)

	return cli.Command{
		Name:  "export",
		Usage: "export evergreen build statistics",
		Flags: addOutputPath(
			cli.IntFlag{
				Name:  numberFlagName,
				Usage: "set the number of revisions for getting makespan",
				Value: 100,
			},
			cli.StringFlag{
				Name:  distroFlagName,
				Usage: "identifier of a configured distro, required for average scheduled-to-start times",
			},
			cli.StringFlag{
				Name:  statFlagName,
				Usage: "description of the type of stats to report, host (for utilization), avg (for average scheduled-to-start) or makespan (for makespan ratios)",
			},
			cli.IntFlag{
				Name:  daysFlagName,
				Value: 1,
				Usage: "set number of days defaults to 1, max of 30 days back",
			},
			cli.StringFlag{
				Name:  granularityFlagName,
				Value: granularityHours,
				Usage: "granularity of data, defaults to hours, may be days, hours, minutes, or second",
			},
			cli.BoolFlag{
				Name:  jsonFlagName,
				Usage: "write output in json, otherwise uses CSV",
			}),
		Before: mergeBeforeFuncs(
			requireStringFlag(statFlagName),
			requireStringValueChoices(granularityFlagName,
				[]string{granularityDays, granularityHours, granularityMinute, granularitySeconds}),
			requiredStringValueChoices(statFlagName,
				[]string{statHostUtilization, statSecheduledToStart, statOptimalMakespan}),
			requireIntValueBetween(daysFlagName, 1, 30),
			requireStringFlag(pathFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.Parent().String(confFlagName)
			isCSV := c.Bool(jsonFlagName)
			statType := c.String(statFlagName)
			granularity := c.String(granularityFlagName)
			days := c.Int(daysFlagName)
			number := c.Int(numberFlagName)
			distro := c.String(distroFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			_ = conf.GetRestCommunicator(ctx)

			_, rc, err := conf.getLegacyClients()
			if err != nil {
				return errors.Wrap(err, "problem accessing evergreen service")
			}

			var body io.ReadCloser
			var granSeconds int

			switch statType {
			case statHostUtilization:
				granSeconds, err = convertGranularityToSeconds(granularity)
				if err != nil {
					return err
				}
				body, err = rc.GetHostUtilizationStats(granSeconds, days, isCSV)
				if err != nil {
					return err
				}
			case statSecheduledToStart:
				if ec.DistroId == "" {
					return errors.New("cannot have empty distro id")
				}
				granSeconds, err = convertGranularityToSeconds(granularity)
				if err != nil {
					return err
				}
				body, err = rc.GetAverageSchedulerStats(granSeconds, days, distro, isCSV)
				if err != nil {
					return err
				}
			case statOptimalMakespan:
				body, err = rc.GetOptimalMakespans(numbers, isCSV)
				if err != nil {
					return err
				}
			default:
				// this should be unreachable:
				return errors.Errorf("%v is not a valid stats type", statType)
			}

			return util.WriteToFile(body, ec.Filepath)
		},
	}
}

// convertGranularityToSeconds takes in a string granularity and returns its
func convertGranularityToSeconds(granString string) (int, error) {
	switch granString {
	case granularityDays:
		return 24 * 60 * 60, nil
	case granularityHours:
		return 60 * 60, nil
	case granularityMinutes:
		return 60, nil
	case granularitySeconds:
		return 1, nil
	default:
		return 0, errors.Errorf("not a valid granularity, %v", granString)
	}
}
