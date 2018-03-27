// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package model_test

import (
	"encoding/json"
	"fmt"

	"testing"

	"io/ioutil"
	"path"

	"github.com/mongodb/mongo-go-driver/mongo/connstring"
	"github.com/mongodb/mongo-go-driver/mongo/internal"
	"github.com/mongodb/mongo-go-driver/mongo/internal/testutil/helpers"
	"github.com/mongodb/mongo-go-driver/mongo/model"
	"github.com/stretchr/testify/require"
)

type response struct {
	Host     string
	IsMaster *internal.IsMasterResult
}

type server struct {
	Type    string
	SetName string
}

type outcome struct {
	Servers      map[string]server
	TopologyType string
	SetName      string
	Compatible   *bool
}

type phase struct {
	Responses []response
	Outcome   outcome
}

type testCase struct {
	Description string
	URI         string
	Phases      []phase
}

const testsDir string = "../../data/server-discovery-and-monitoring/"

func (r *response) UnmarshalJSON(buf []byte) error {
	tmp := []interface{}{&r.Host, &r.IsMaster}
	if err := json.Unmarshal(buf, &tmp); err != nil {
		return err
	}

	if len(tmp) != 2 {
		return fmt.Errorf("'response' JSON array must have exactly two elements")
	}

	return nil
}

func setUpFSM(t *testing.T, uri string) *model.FSM {
	fsm := model.NewFSM()

	cs, err := connstring.Parse(uri)
	require.NoError(t, err)

	fsm.SetName = cs.ReplicaSet
	if fsm.SetName != "" {
		fsm.Kind = model.ReplicaSetNoPrimary
	} else if len(cs.Hosts) == 1 {
		fsm.Kind = model.Single
	}

	for _, host := range cs.Hosts {
		fsm.Servers = append(fsm.Servers, &model.Server{Addr: model.Addr(host).Canonicalize()})
	}

	return fsm
}

func applyResponses(fsm *model.FSM, responses []response) error {
	for _, response := range responses {
		server := model.BuildServer(model.Addr(response.Host), response.IsMaster, nil)
		err := fsm.Apply(server)

		if err != nil {
			return err
		}
	}

	return nil
}

func runTest(t *testing.T, directory string, filename string) {
	filepath := path.Join(testsDir, directory, filename)
	content, err := ioutil.ReadFile(filepath)
	require.NoError(t, err)

	// Remove ".json" from filename.
	filename = filename[:len(filename)-5]
	testName := directory + "/" + filename + ":"

	t.Run(testName, func(t *testing.T) {
		var test testCase
		require.NoError(t, json.Unmarshal(content, &test))
		fsm := setUpFSM(t, test.URI)

		for _, phase := range test.Phases {
			err = applyResponses(fsm, phase.Responses)
			if phase.Outcome.Compatible == nil || *phase.Outcome.Compatible {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				continue
			}

			require.Equal(t, phase.Outcome.TopologyType, fsm.Kind.String())
			require.Equal(t, phase.Outcome.SetName, fsm.SetName)
			require.Equal(t, len(phase.Outcome.Servers), len(fsm.Servers))

			for addr, server := range phase.Outcome.Servers {
				fsmServer, ok := fsm.Server(model.Addr(addr))
				require.True(t, ok)

				require.Equal(t, model.Addr(addr), fsmServer.Addr)
				require.Equal(t, server.SetName, fsmServer.SetName)

				// PossiblePrimary is only relevant to single-threaded drivers.
				if server.Type == "PossiblePrimary" {
					server.Type = "Unknown"
				}

				require.Equal(t, server.Type, fsmServer.Kind.String())
			}
		}
	})
}

// Test case for all SDAM spec tests.
func TestSDAMSpec(t *testing.T) {
	for _, subdir := range []string{"single", "rs", "sharded"} {
		for _, file := range testhelpers.FindJSONFilesInDir(t, path.Join(testsDir, subdir)) {
			runTest(t, subdir, file)
		}
	}
}
