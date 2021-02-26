package cli

import (
	"context"

	"github.com/mongodb/jasper/remote"
	"github.com/urfave/cli"
)

// Constants representing the jasper.LoggingCache interface as CLI commands.
const (
	LoggingCacheCommand               = "logging-cache"
	LoggingCacheCreateCommand         = "create"
	LoggingCacheGetCommand            = "get"
	LoggingCacheRemoveCommand         = "remove"
	LoggingCacheCloseAndRemoveCommand = "close-and-remove"
	LoggingCacheClearCommand          = "clear"
	LoggingCachePruneCommand          = "prune"
	LoggingCacheLenCommand            = "len"
)

// LoggingCache creates a cli.Command that supports the jasper.LoggingCache
// interface. (jasper.LoggingCache).Put is not supported as there is no CLI
// equivalent.
func LoggingCache() cli.Command {
	return cli.Command{
		Name: LoggingCacheCommand,
		Subcommands: []cli.Command{
			loggingCacheCreate(),
			loggingCacheGet(),
			loggingCacheRemove(),
			loggingCacheCloseAndRemove(),
			loggingCacheClear(),
			loggingCachePrune(),
			loggingCacheLen(),
		},
	}
}

func loggingCacheCreate() cli.Command {
	return cli.Command{
		Name:   LoggingCacheCreateCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := LoggingCacheCreateInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				lc := client.LoggingCache(ctx)
				if lc == nil {
					return &CachedLoggerResponse{OutcomeResponse: *makeOutcomeResponse(remote.ErrLoggingCacheNotSupported)}
				}
				logger, err := lc.Create(input.ID, &input.Output)
				if err != nil {
					return &CachedLoggerResponse{OutcomeResponse: *makeOutcomeResponse(err)}
				}
				return &CachedLoggerResponse{Logger: *logger, OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func loggingCacheGet() cli.Command {
	return cli.Command{
		Name:   LoggingCacheGetCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := IDInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				lc := client.LoggingCache(ctx)
				if lc == nil {
					return &CachedLoggerResponse{OutcomeResponse: *makeOutcomeResponse(remote.ErrLoggingCacheNotSupported)}
				}
				logger, err := lc.Get(input.ID)
				if err != nil {
					return &CachedLoggerResponse{OutcomeResponse: *makeOutcomeResponse(err)}
				}
				return &CachedLoggerResponse{Logger: *logger, OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func loggingCacheRemove() cli.Command {
	return cli.Command{
		Name:   LoggingCacheRemoveCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := IDInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				lc := client.LoggingCache(ctx)
				if lc == nil {
					return makeOutcomeResponse(remote.ErrLoggingCacheNotSupported)
				}
				err := lc.Remove(input.ID)
				return makeOutcomeResponse(err)
			})
		},
	}
}

func loggingCacheCloseAndRemove() cli.Command {
	return cli.Command{
		Name:   LoggingCacheCloseAndRemoveCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := IDInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				lc := client.LoggingCache(ctx)
				if lc == nil {
					return makeOutcomeResponse(remote.ErrLoggingCacheNotSupported)
				}
				err := lc.CloseAndRemove(ctx, input.ID)
				return makeOutcomeResponse(err)
			})
		},
	}
}

func loggingCacheClear() cli.Command {
	return cli.Command{
		Name:   LoggingCacheClearCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			return doPassthroughOutput(c, func(ctx context.Context, client remote.Manager) interface{} {
				lc := client.LoggingCache(ctx)
				if lc == nil {
					return makeOutcomeResponse(remote.ErrLoggingCacheNotSupported)
				}
				err := lc.Clear(ctx)
				return makeOutcomeResponse(err)
			})
		},
	}
}

func loggingCachePrune() cli.Command {
	return cli.Command{
		Name:   LoggingCachePruneCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := LoggingCachePruneInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				lc := client.LoggingCache(ctx)
				if lc == nil {
					return makeOutcomeResponse(remote.ErrLoggingCacheNotSupported)
				}
				err := lc.Prune(input.LastAccessed)
				return makeOutcomeResponse(err)
			})
		},
	}
}

func loggingCacheLen() cli.Command {
	return cli.Command{
		Name:   LoggingCacheLenCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			return doPassthroughOutput(c, func(ctx context.Context, client remote.Manager) interface{} {
				lc := client.LoggingCache(ctx)
				if lc == nil {
					return &LoggingCacheLenResponse{OutcomeResponse: *makeOutcomeResponse(remote.ErrLoggingCacheNotSupported)}
				}
				length, err := lc.Len()
				return &LoggingCacheLenResponse{Len: length, OutcomeResponse: *makeOutcomeResponse(err)}
			})
		},
	}
}
