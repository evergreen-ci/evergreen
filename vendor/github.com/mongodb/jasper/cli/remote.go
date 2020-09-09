package cli

import (
	"context"

	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Constants representing the remote.Manager interface as CLI commands.
const (
	RemoteCommand             = "remote"
	ConfigureCacheCommand     = "configure-cache"
	DownloadFileCommand       = "download-file"
	DownloadMongoDBCommand    = "download-mongodb"
	GetBuildloggerURLsCommand = "get-buildlogger-urls"
	GetLogStreamCommand       = "get-log-stream"
	SignalEventCommand        = "signal-event"
	SendMessagesCommand       = "send-messages"
	CreateScriptingCommand    = "create-scripting"
	GetScriptingCommand       = "get-scripting"
)

// Remote creates a cli.Command that supports the remote-specific methods in the
// remote.Manager interface except for CloseClient, for which there is no CLI
// equivalent.
func Remote() cli.Command {
	return cli.Command{
		Name: RemoteCommand,
		Subcommands: []cli.Command{
			remoteConfigureCache(),
			remoteDownloadFile(),
			remoteDownloadMongoDB(),
			remoteGetLogStream(),
			remoteGetBuildloggerURLs(),
			remoteSignalEvent(),
			remoteSendMessages(),
			remoteCreateScripting(),
			remoteGetScripting(),
		},
	}
}
func remoteConfigureCache() cli.Command {
	return cli.Command{
		Name:   ConfigureCacheCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := options.Cache{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				return makeOutcomeResponse(client.ConfigureCache(ctx, input))
			})
		},
	}
}

func remoteDownloadFile() cli.Command {
	return cli.Command{
		Name:   DownloadFileCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := options.Download{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				return makeOutcomeResponse(client.DownloadFile(ctx, input))
			})
		},
	}
}

func remoteDownloadMongoDB() cli.Command {
	return cli.Command{
		Name:   DownloadMongoDBCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := options.MongoDBDownload{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				return makeOutcomeResponse(client.DownloadMongoDB(ctx, input))
			})
		},
	}
}

func remoteGetLogStream() cli.Command {
	return cli.Command{
		Name:   GetLogStreamCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := LogStreamInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				logs, err := client.GetLogStream(ctx, input.ID, input.Count)
				if err != nil {
					return &LogStreamResponse{OutcomeResponse: *makeOutcomeResponse(err)}
				}
				return &LogStreamResponse{LogStream: logs, OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func remoteGetBuildloggerURLs() cli.Command {
	return cli.Command{
		Name:   GetBuildloggerURLsCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := IDInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				urls, err := client.GetBuildloggerURLs(ctx, input.ID)
				if err != nil {
					return &BuildloggerURLsResponse{OutcomeResponse: *makeOutcomeResponse(err)}
				}
				return &BuildloggerURLsResponse{URLs: urls, OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func remoteSignalEvent() cli.Command {
	return cli.Command{
		Name:   SignalEventCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := EventInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				if err := client.SignalEvent(ctx, input.Name); err != nil {
					return makeOutcomeResponse(err)
				}
				return makeOutcomeResponse(nil)
			})
		},
	}
}

func remoteSendMessages() cli.Command {
	return cli.Command{
		Name:   SendMessagesCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := options.LoggingPayload{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				if err := client.SendMessages(ctx, input); err != nil {
					return makeOutcomeResponse(err)
				}
				return makeOutcomeResponse(nil)
			})
		},
	}
}

func remoteCreateScripting() cli.Command {
	return cli.Command{
		Name:   CreateScriptingCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			in := &ScriptingCreateInput{}
			return doPassthroughInputOutput(c, in, func(ctx context.Context, client remote.Manager) interface{} {
				harnessOpts, err := in.Export()
				if err != nil {
					return &IDResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error creating scripting harness"))}
				}

				env, err := client.CreateScripting(ctx, harnessOpts)
				if err != nil {
					return &IDResponse{ID: harnessOpts.ID(), OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error creating scripting harness"))}
				}
				return &IDResponse{ID: env.ID(), OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func remoteGetScripting() cli.Command {
	return cli.Command{
		Name:   GetScriptingCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &IDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client remote.Manager) interface{} {
				_, err := client.GetScripting(ctx, input.ID)
				if err != nil {
					return makeOutcomeResponse(errors.Wrapf(err, "getting scripting harness with ID '%s'", input.ID))
				}
				return makeOutcomeResponse(nil)
			})
		},
	}
}
