package remote

import (
	"io/ioutil"

	"github.com/pkg/errors"

	"golang.org/x/crypto/ssh"
)

// Given a path to a file containing a PEM-encoded private key,
// read in the file and use the private key to create an ssh authenticator.
func authFromPrivKeyFile(file string) ([]ssh.AuthMethod, error) {

	// read in the file
	fileBytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading private key file `%v`", file)
	}

	// convert it to an ssh.Signer
	signer, err := ssh.ParsePrivateKey(fileBytes)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing private key from file `%v`", file)
	}

	return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
}

// Create a client config, using the appropriate user and PEM-encoded private
// key file.
func createClientConfig(user string, keyfile string) (*ssh.ClientConfig, error) {

	// initialize the config, with the correct user but no authentication
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{},
	}

	// read in the keyfile, if specified, and set up authentication based on it
	if keyfile != "" {
		authMethods, err := authFromPrivKeyFile(keyfile)
		if err != nil {
			return nil, errors.Wrapf(err, "error using private key from file `%v`", keyfile)
		}
		config.Auth = authMethods
	}

	return config, nil
}
