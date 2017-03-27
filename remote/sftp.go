package remote

import (
	"github.com/pkg/errors"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPGateway wraps an SFTP client.
type SFTPGateway struct {

	// the sftp client that the gateway is managing
	Client *sftp.Client

	// the remote host
	Host string
	// the user we'll access the file as on the remote machine
	User string

	// the file containing the private key we'll use to connect
	Keyfile string
}

// Connect to the other side, and initialize the SFTP client.
func (gateway *SFTPGateway) Init() error {

	// configure appropriately
	clientConfig, err := createClientConfig(gateway.User, gateway.Keyfile)
	if err != nil {
		return errors.Wrap(err, "error configuring ssh")
	}

	// connect to the other side
	conn, err := ssh.Dial("tcp", gateway.Host, clientConfig)
	if err != nil {
		return errors.Wrapf(err, "error connecting to ssh server at `%v`", gateway.Host)
	}

	// create the sftp client
	gateway.Client, err = sftp.NewClient(conn)
	if err != nil {
		return errors.Wrapf(err, "error creating sftp client to `%v`", gateway.Host)
	}

	return nil

}

// Close frees any necessary resources.
func (gateway *SFTPGateway) Close() error {
	return errors.WithStack(gateway.Client.Close())
}
