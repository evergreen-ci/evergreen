// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package cluster

import (
	"bytes"
	"strings"

	"github.com/mongodb/mongo-go-driver/mongo/connstring"
	"github.com/mongodb/mongo-go-driver/mongo/private/auth"
	"github.com/mongodb/mongo-go-driver/mongo/private/conn"
	"github.com/mongodb/mongo-go-driver/mongo/private/server"
)

func newConfig(opts ...Option) (*config, error) {
	cfg := &config{
		seedList: []string{"localhost:27017"},
	}

	err := cfg.apply(opts...)
	return cfg, err
}

// Option configures a cluster.
type Option func(*config) error

type config struct {
	mode           MonitorMode
	replicaSetName string
	seedList       []string
	serverOpts     []server.Option
}

func (c *config) reconfig(opts ...Option) (*config, error) {
	cfg := &config{
		mode:           c.mode,
		replicaSetName: c.replicaSetName,
		seedList:       c.seedList,
		serverOpts:     c.serverOpts,
	}

	err := cfg.apply(opts...)
	return cfg, err
}

func (c *config) apply(opts ...Option) error {
	for _, opt := range opts {
		err := opt(c)
		if err != nil {
			return err
		}
	}

	return nil
}

// WithConnString configures the cluster using the connection
// string.
func WithConnString(cs connstring.ConnString) Option {
	return func(c *config) error {
		var connOpts []conn.Option

		if cs.AppName != "" {
			connOpts = append(connOpts, conn.WithAppName(cs.AppName))
		}

		switch cs.Connect {
		case connstring.SingleConnect:
			c.mode = SingleMode
		}

		c.seedList = cs.Hosts

		if cs.ConnectTimeout > 0 {
			connOpts = append(connOpts, conn.WithConnectTimeout(cs.ConnectTimeout))
		}

		if cs.HeartbeatInterval > 0 {
			c.serverOpts = append(c.serverOpts, server.WithHeartbeatInterval(cs.HeartbeatInterval))
		}

		if cs.MaxConnIdleTime > 0 {
			connOpts = append(connOpts, conn.WithIdleTimeout(cs.MaxConnIdleTime))
		}

		if cs.MaxConnLifeTime > 0 {
			connOpts = append(connOpts, conn.WithIdleTimeout(cs.MaxConnLifeTime))
		}

		if cs.MaxConnsPerHostSet {
			c.serverOpts = append(c.serverOpts, server.WithMaxConnections(cs.MaxConnsPerHost))
		}

		if cs.MaxIdleConnsPerHostSet {
			c.serverOpts = append(c.serverOpts, server.WithMaxIdleConnections(cs.MaxIdleConnsPerHost))
		}

		if cs.ReplicaSet != "" {
			c.replicaSetName = cs.ReplicaSet
		}

		var x509Username string
		if cs.SSL {
			tls := conn.NewTLSConfig()

			if cs.SSLCaFileSet {
				err := tls.AddCaCertFromFile(cs.SSLCaFile)
				if err != nil {
					return err
				}
			}

			if cs.SSLInsecure {
				tls.SetInsecure(true)
			}

			if cs.SSLClientCertificateKeyFileSet {
				s, err := tls.AddClientCertFromFile(cs.SSLClientCertificateKeyFile)
				if err != nil {
					return err
				}

				// The Go x509 package gives the subject with the pairs in reverse order that we want.
				pairs := strings.Split(s, ",")
				b := bytes.NewBufferString("")

				for i := len(pairs) - 1; i >= 0; i-- {
					b.WriteString(pairs[i])

					if i > 0 {
						b.WriteString(",")
					}
				}

				x509Username = b.String()
			}

			connOpts = append(connOpts, conn.WithTLSConfig(tls))
		}

		if cs.Username != "" || cs.AuthMechanism == auth.MongoDBX509 || cs.AuthMechanism == auth.GSSAPI {
			cred := &auth.Cred{
				Source:      "admin",
				Username:    cs.Username,
				Password:    cs.Password,
				PasswordSet: cs.PasswordSet,
				Props:       cs.AuthMechanismProperties,
			}

			if cs.AuthSource != "" {
				cred.Source = cs.AuthSource
			} else {
				switch cs.AuthMechanism {
				case auth.MongoDBX509:
					if cred.Username == "" {
						cred.Username = x509Username
					}
					fallthrough
				case auth.GSSAPI, auth.PLAIN:
					cred.Source = "$external"
				default:
					cred.Source = cs.Database
				}
			}

			authenticator, err := auth.CreateAuthenticator(cs.AuthMechanism, cred)
			if err != nil {
				return err
			}

			c.serverOpts = append(
				c.serverOpts,
				server.WithWrappedConnectionOpener(func(current conn.Opener) conn.Opener {
					return auth.Opener(current, authenticator)
				}),
			)
		}

		if len(connOpts) > 0 {
			c.serverOpts = append(c.serverOpts, server.WithMoreConnectionOptions(connOpts...))
		}

		return nil
	}
}

// WithMode configures the cluster's monitor mode.
// This option will be ignored when the cluster is created with a
// pre-existing monitor.
func WithMode(mode MonitorMode) Option {
	return func(c *config) error {
		c.mode = mode
		return nil
	}
}

// WithReplicaSetName configures the cluster's default replica set name.
// This option will be ignored when the cluster is created with a
// pre-existing monitor.
func WithReplicaSetName(name string) Option {
	return func(c *config) error {
		c.replicaSetName = name
		return nil
	}
}

// WithSeedList configures a cluster's seed list.
// This option will be ignored when the cluster is created with a
// pre-existing monitor.
func WithSeedList(seedList ...string) Option {
	return func(c *config) error {
		c.seedList = seedList
		return nil
	}
}

// WithServerOptions configures a cluster's server options for
// when a new server needs to get created. The options provided
// overwrite all previously configured options.
func WithServerOptions(opts ...server.Option) Option {
	return func(c *config) error {
		c.serverOpts = opts
		return nil
	}
}

// WithMoreServerOptions configures a cluster's server options for
// when a new server needs to get created. The options provided are
// appended to any current options and may override previously
// configured options.
func WithMoreServerOptions(opts ...server.Option) Option {
	return func(c *config) error {
		c.serverOpts = append(c.serverOpts, opts...)
		return nil
	}
}
