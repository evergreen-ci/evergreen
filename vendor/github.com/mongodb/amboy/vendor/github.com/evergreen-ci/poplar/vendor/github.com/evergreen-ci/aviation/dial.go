package aviation

import (
	"context"
	"crypto/tls"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
}

func (opts *DialOptions) validate() error {
	if opts.Address == "" {
		return errors.New("must provide rpc address")
	}
	if opts.TLSConf == nil && (opts.CAFile == "") != (opts.CrtFile == "") != (opts.KeyFile == "") {
		return errors.New("must provide all or none of the required certificate filenames")
	}

	return nil
}

func (opts *DialOptions) getOpts() ([]grpc.DialOption, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	dialOpts := []grpc.DialOption{}

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
			return nil, errors.Wrap(err, "problem getting client TLS config")
		}
	}
	if tlsConf != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	}

	return dialOpts, nil
}

// Dial creates a client connection to a RPC service via gRPC.
func Dial(ctx context.Context, opts DialOptions) (*grpc.ClientConn, error) {
	dialOpts, err := opts.getOpts()
	if err != nil {
		return nil, errors.Wrap(err, "problem getting gRPC dial options")
	}

	conn, err := grpc.DialContext(ctx, opts.Address, dialOpts...)
	return conn, errors.Wrap(err, "problem dialing rpc")
}
