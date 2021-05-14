package aviation

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// DialOptions describes the options for creating a client connection to a RPC
// service via gRPC.
type DialOptions struct {
	// Address is the RPC address to connect to.
	Address string
	// Retries specifies the number of times the client connection retries
	// an operation before failing.
	Retries int
	// TLSConf is the config for TLS authentication. If TLSConf is
	// specified, CAFile, CrtFile, and KeyFile are ignored. If neither
	// TLSFile nor the certificate files are specified, the connection is
	// created without TLS. (Optional)
	TLSConf *tls.Config
	// CAFile is the name of the file with the CA certificate for TLS. If
	// specified, CrtFile and KeyFile must also be specified. (Optional)
	CAFile string
	// CrtFile is the name of the file with the user certificate for TLS.
	// If specified, CAFile and KeyFile must also be specified. (Optional)
	CrtFile string
	// KeyFile is the name of the file with the key certificate for TLS. If
	// specified, CAFile and CrtFile must also be specified. (Optional)
	KeyFile string
	// Username is the username of the API user. If specified, APIKey must
	// also be specified. (Optional)
	Username string
	// APIKey is the API key for user authentication. If specified,
	// Username must also be specified. (Optional)
	APIKey string
	// APIUserHeader is the metadata key for the requester's username. This
	// must be specified if Username and APIKey are specified. (Optional)
	APIUserHeader string
	// APIKeyHeader is the metadata key for the requester's API key. This
	// must be specified if Username and APIKey are specified. (Optional)
	APIKeyHeader string
}

func (opts *DialOptions) validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.AddWhen(opts.Address == "", errors.New("must provide rpc address"))
	catcher.AddWhen(
		((opts.CAFile == "") != (opts.CrtFile == "")) || ((opts.CAFile == "") != (opts.KeyFile == "")),
		errors.New("must provide all or none of the required certificate filenames"),
	)
	catcher.AddWhen((opts.Username == "") != (opts.APIKey == ""), errors.New("must provide both a username and API key or neither"))
	catcher.AddWhen(
		(opts.Username != "" || opts.APIKey != "") && (opts.APIUserHeader == "" || opts.APIKeyHeader == ""),
		errors.New("must provide an API user header and key header when providing a username and API key"),
	)

	return catcher.Resolve()
}

func (opts *DialOptions) getOpts() ([]grpc.DialOption, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	dialOpts := []grpc.DialOption{
		// TODO (PM-2158): After upgrading Go, we should investiage
		// whether later versions of grpc fixed the following issue
		// and, if so, upgrade grpc:
		// Even though we use the default keep alive time of infinity
		// (meaning the client should never send a keep alive ping), we
		// need a large keep alive timeout for longer running grpc
		// requests and streams. This is likely a bug in the grpc code.
		grpc.WithKeepaliveParams(keepalive.ClientParameters{Timeout: 1200 * time.Second}),
	}

	if opts.Retries > 0 {
		dialOpts = append(
			dialOpts,
			grpc.WithUnaryInterceptor(MakeRetryUnaryClientInterceptor(opts.Retries)),
			grpc.WithStreamInterceptor(MakeRetryStreamClientInterceptor(opts.Retries)),
		)
	}

	tlsConf := opts.TLSConf
	if tlsConf == nil && opts.CAFile != "" {
		var err error
		tlsConf, err = GetClientTLSConfigFromFiles(opts.CAFile, opts.CrtFile, opts.KeyFile)
		if err != nil {
			return nil, errors.Wrap(err, "getting client TLS config")
		}
	}
	if tlsConf != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}

	if opts.Username != "" {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(&userAuth{
			username:   opts.Username,
			apiKey:     opts.APIKey,
			userHeader: opts.APIUserHeader,
			keyHeader:  opts.APIKeyHeader,
			tls:        tlsConf != nil,
		}))
	}

	return dialOpts, nil
}

// Dial creates a client connection to a RPC service via gRPC.
func Dial(ctx context.Context, opts DialOptions) (*grpc.ClientConn, error) {
	dialOpts, err := opts.getOpts()
	if err != nil {
		return nil, errors.Wrap(err, "getting gRPC dial options")
	}

	conn, err := grpc.DialContext(ctx, opts.Address, dialOpts...)
	return conn, errors.Wrap(err, "dialing rpc")
}

type userAuth struct {
	username   string
	apiKey     string
	userHeader string
	keyHeader  string
	tls        bool
}

func (a *userAuth) RequireTransportSecurity() bool { return a.tls }
func (a *userAuth) GetRequestMetadata(ctx context.Context, in ...string) (map[string]string, error) {
	return map[string]string{
		a.userHeader: a.username,
		a.keyHeader:  a.apiKey,
	}, nil
}
