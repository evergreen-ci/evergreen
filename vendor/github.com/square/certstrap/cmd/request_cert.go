/*-
 * Copyright 2015 Square Inc.
 * Copyright 2014 CoreOS
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/square/certstrap/depot"
	"github.com/square/certstrap/pkix"
	"github.com/urfave/cli"
)

// NewCertRequestCommand sets up a "request-cert" command to create a request for a new certificate (CSR)
func NewCertRequestCommand() cli.Command {
	return cli.Command{
		Name:        "request-cert",
		Usage:       "Create certificate request for host",
		Description: "Create certificate for host, including certificate signing request and key. Must sign the request in order to generate a certificate.",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "passphrase",
				Usage: "Passphrase to encrypt private-key PEM block",
			},
			cli.IntFlag{
				Name:  "key-bits",
				Value: 2048,
				Usage: "Size (in bits) of RSA keypair to generate (example: 4096)",
			},
			cli.StringFlag{
				Name:  "organization, o",
				Usage: "Sets the Organization (O) field of the certificate",
			},
			cli.StringFlag{
				Name:  "country, c",
				Usage: "Sets the Country (C) field of the certificate",
			},
			cli.StringFlag{
				Name:  "locality, l",
				Usage: "Sets the Locality (L) field of the certificate",
			},
			cli.StringFlag{
				Name:  "common-name, cn",
				Usage: "Sets the Common Name (CN) field of the certificate",
			},
			cli.StringFlag{
				Name:  "organizational-unit, ou",
				Usage: "Sets the Organizational Unit (OU) field of the certificate",
			},
			cli.StringFlag{
				Name:  "province, st",
				Usage: "Sets the State/Province (ST) field of the certificate",
			},
			cli.StringFlag{
				Name:  "ip",
				Usage: "IP addresses to add as subject alt name (comma separated)",
			},
			cli.StringFlag{
				Name:  "domain",
				Usage: "DNS entries to add as subject alt name (comma separated)",
			},
			cli.StringFlag{
				Name:  "uri",
				Usage: "URI values to add as subject alt name (comma separated)",
			},
			cli.StringFlag{
				Name:  "key",
				Usage: "Path to private key PEM file (if blank, will generate new keypair)",
			},
			cli.BoolFlag{
				Name:  "stdout",
				Usage: "Print signing request to stdout in addition to saving file",
			},
		},
		Action: newCertAction,
	}
}

func newCertAction(c *cli.Context) {
	var name = ""
	var err error

	// The CLI Context returns an empty string ("") if no value is available
	ips, err := pkix.ParseAndValidateIPs(c.String("ip"))

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// The CLI Context returns an empty string ("") if no value is available
	uris, err := pkix.ParseAndValidateURIs(c.String("uri"))

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	domains := strings.Split(c.String("domain"), ",")
	if c.String("domain") == "" {
		domains = nil
	}

	switch {
	case len(c.String("common-name")) != 0:
		name = c.String("common-name")
	case len(domains) != 0:
		name = domains[0]
	default:
		fmt.Fprintln(os.Stderr, "Must provide Common Name or domain")
		os.Exit(1)
	}

	var formattedName = formatName(name)

	if depot.CheckCertificateSigningRequest(d, formattedName) || depot.CheckPrivateKey(d, formattedName) {
		fmt.Fprintf(os.Stderr, "Certificate request \"%s\" already exists!\n", formattedName)
		os.Exit(1)
	}

	var passphrase []byte
	if c.IsSet("passphrase") {
		passphrase = []byte(c.String("passphrase"))
	} else {
		passphrase, err = createPassPhrase()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	var key *pkix.Key
	if c.IsSet("key") {
		keyBytes, err := ioutil.ReadFile(c.String("key"))
		key, err = pkix.NewKeyFromPrivateKeyPEM(keyBytes)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Read Key error:", err)
			os.Exit(1)
		}
		fmt.Printf("Read %s.key\n", name)
	} else {
		key, err = pkix.CreateRSAKey(c.Int("key-bits"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "Create RSA Key error:", err)
			os.Exit(1)
		}
		if len(passphrase) > 0 {
			fmt.Printf("Created %s/%s.key (encrypted by passphrase)\n", depotDir, formattedName)
		} else {
			fmt.Printf("Created %s/%s.key\n", depotDir, formattedName)
		}
	}

	csr, err := pkix.CreateCertificateSigningRequest(key, c.String("organizational-unit"), ips, domains, uris, c.String("organization"), c.String("country"), c.String("province"), c.String("locality"), name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Create certificate request error:", err)
		os.Exit(1)
	} else {
		fmt.Printf("Created %s/%s.csr\n", depotDir, formattedName)
	}

	if c.Bool("stdout") {
		csrBytes, err := csr.Export()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Print certificate request error:", err)
			os.Exit(1)
		} else {
			fmt.Printf(string(csrBytes))
		}
	}

	if err = depot.PutCertificateSigningRequest(d, formattedName, csr); err != nil {
		fmt.Fprintln(os.Stderr, "Save certificate request error:", err)
	}
	if len(passphrase) > 0 {
		if err = depot.PutEncryptedPrivateKey(d, formattedName, key, passphrase); err != nil {
			fmt.Fprintln(os.Stderr, "Save encrypted private key error:", err)
		}
	} else {
		if err = depot.PutPrivateKey(d, formattedName, key); err != nil {
			fmt.Fprintln(os.Stderr, "Save private key error:", err)
		}
	}
}

func formatName(name string) string {
	var filenameAcceptable, err = regexp.Compile("[^a-zA-Z0-9._-]")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error compiling regex:", err)
		os.Exit(1)
	}
	return string(filenameAcceptable.ReplaceAll([]byte(name), []byte("_")))
}
