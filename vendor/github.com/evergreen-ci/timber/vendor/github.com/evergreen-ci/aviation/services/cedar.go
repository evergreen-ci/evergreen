package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/aviation"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// API headers for the cedar service.
const (
	APIUserHeader = "Api-User"
	APIKeyHeader  = "Api-Key"
)

// DialCedarOptions describes the options for the DialCedar function. The base
// address defaults to `cedar.mongodb.com` and the RPC port to 7070. If a base
// address is provided the RPC port must also be provided. The username and API
// key must always be provided.
type DialCedarOptions struct {
	BaseAddress string
	RPCPort     string
	Username    string
	APIKey      string
	CACerts     [][]byte
	TLSAuth     bool
	Insecure    bool
	Retries     int
}

func (opts *DialCedarOptions) validate() error {
	catcher := grip.NewBasicCatcher()

	if opts.BaseAddress == "" {
		opts.BaseAddress = "cedar.mongodb.com"
		opts.RPCPort = "7070"
	}

	catcher.NewWhen(opts.Username == "" || opts.APIKey == "", "must provide username and API key")
	catcher.NewWhen(opts.RPCPort == "", "must provide the RPC port")
	catcher.NewWhen(opts.TLSAuth && opts.Insecure, "cannot use TLS auth over an insecure connection")
	catcher.NewWhen(len(opts.CACerts) > 0 && opts.Insecure, "cannot use CA certificates over an insecure connection")

	return catcher.Resolve()
}

type userCredentials struct {
	Username string `json:"username"`
	apiKey   string
}

// DialCedar is a convenience function for creating a RPC client connection
// with cedar via gRPC.
func DialCedar(ctx context.Context, client *http.Client, opts *DialCedarOptions) (*grpc.ClientConn, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid dial cedar options")
	}

	var tlsConf *tls.Config
	if opts.TLSAuth {
		httpAddress := "https://" + opts.BaseAddress
		creds := &userCredentials{
			Username: opts.Username,
			apiKey:   opts.APIKey,
		}

		ca, err := makeCedarCertRequest(ctx, client, http.MethodGet, httpAddress+"/rest/v1/admin/ca", nil)
		if err != nil {
			return nil, errors.Wrap(err, "getting cedar root cert")
		}
		crt, err := makeCedarCertRequest(ctx, client, http.MethodPost, httpAddress+"/rest/v1/admin/users/certificate", creds)
		if err != nil {
			return nil, errors.Wrap(err, "getting cedar user cert")
		}
		key, err := makeCedarCertRequest(ctx, client, http.MethodPost, httpAddress+"/rest/v1/admin/users/certificate/key", creds)
		if err != nil {
			return nil, errors.Wrap(err, "getting cedar user key")
		}

		tlsConf, err = aviation.GetClientTLSConfig(append(opts.CACerts, ca), crt, key)
		if err != nil {
			return nil, errors.Wrap(err, "creating TLS config")
		}
	} else if !opts.Insecure {
		cp, err := aviation.GetCACertPool(opts.CACerts...)
		if err != nil {
			return nil, errors.Wrap(err, "creating CA cert pool")
		}
		tlsConf = &tls.Config{RootCAs: cp}
	}

	return aviation.Dial(ctx, aviation.DialOptions{
		Address:       opts.BaseAddress + ":" + opts.RPCPort,
		Retries:       opts.Retries,
		Username:      opts.Username,
		APIKey:        opts.APIKey,
		APIUserHeader: APIUserHeader,
		APIKeyHeader:  APIKeyHeader,
		TLSConf:       tlsConf,
	})
}

func makeCedarCertRequest(ctx context.Context, client *http.Client, method, url string, creds *userCredentials) ([]byte, error) {
	var body io.Reader
	if creds != nil {
		payload, err := json.Marshal(creds)
		if err != nil {
			return nil, errors.Wrap(err, "marshalling credentials payload")
		}
		body = bytes.NewBuffer(payload)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, errors.Wrap(err, "creating http request")
	}
	req = req.WithContext(ctx)

	if creds != nil && creds.Username != "" && creds.apiKey != "" {
		req.Header.Set(APIUserHeader, creds.Username)
		req.Header.Set(APIKeyHeader, creds.apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}
	defer resp.Body.Close()

	out, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response")
	}

	if resp.StatusCode != http.StatusOK {
		return out, errors.Errorf("failed request with status code %d", resp.StatusCode)
	}

	return out, nil
}
