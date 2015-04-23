package remote

import (
	"fmt"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// A wrapper for an sftp client.
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

// Connect to the other side, and initialize the sftp client.
func (gateway *SFTPGateway) Init() error {

	// configure appropriately
	clientConfig, err := createClientConfig(gateway.User, gateway.Keyfile)
	if err != nil {
		return fmt.Errorf("error configuring ssh: %v", err)
	}

	// connect to the other side
	conn, err := ssh.Dial("tcp", gateway.Host, clientConfig)
	if err != nil {
		return fmt.Errorf("error connecting to ssh server at `%v`: %v", gateway.Host, err)
	}

	// create the sftp client
	gateway.Client, err = sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("error creating sftp client to `%v`: %v", gateway.Host, err)
	}

	return nil

}

// Free any necessary resources.
func (gateway *SFTPGateway) Close() error {
	return gateway.Client.Close()
}
