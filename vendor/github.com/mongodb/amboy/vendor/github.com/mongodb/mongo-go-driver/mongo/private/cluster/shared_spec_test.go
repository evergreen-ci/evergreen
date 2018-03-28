// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package cluster_test

import (
	"encoding/json"
	"io/ioutil"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/mongodb/mongo-go-driver/mongo/model"
	"github.com/mongodb/mongo-go-driver/mongo/private/cluster"
	"github.com/mongodb/mongo-go-driver/mongo/readpref"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	TopologyDescription  topDesc       `json:"topology_description"`
	Operation            string        `json:"operation"`
	ReadPreference       readPref      `json:"read_preference"`
	SuitableServers      []*serverDesc `json:"suitable_servers"`
	InLatencyWindow      []*serverDesc `json:"in_latency_window"`
	HeartbeatFrequencyMS *int          `json:"heartbeatFrequencyMS"`
	Error                *bool
}

type topDesc struct {
	Type    string        `json:"type"`
	Servers []*serverDesc `json:"servers"`
}

type serverDesc struct {
	Address        string            `json:"address"`
	AverageRTTMS   *int              `json:"avg_rtt_ms"`
	MaxWireVersion *int32            `json:"maxWireVersion"`
	LastUpdateTime *int              `json:"lastUpdateTime"`
	LastWrite      *lastWriteDate    `json:"lastWrite"`
	Type           string            `json:"type"`
	Tags           map[string]string `json:"tags"`
}

type lastWriteDate struct {
	LastWriteDate lastWriteDateInner `json:"lastWriteDate"`
}

// TODO(GODRIVER-33): Use proper extended JSON parsing to eliminate the need for this struct.
type lastWriteDateInner struct {
	Value string `json:"$numberLong"`
}

type readPref struct {
	MaxStaleness *int                `json:"maxStalenessSeconds"`
	Mode         string              `json:"mode"`
	TagSets      []map[string]string `json:"tag_sets"`
}

func clusterKindFromString(s string) model.ClusterKind {
	switch s {
	case "Single":
		return model.Single
	case "ReplicaSet":
		return model.ReplicaSet
	case "ReplicaSetNoPrimary":
		return model.ReplicaSetNoPrimary
	case "ReplicaSetWithPrimary":
		return model.ReplicaSetWithPrimary
	case "Sharded":
		return model.Sharded
	}

	return model.Unknown
}

func serverKindFromString(s string) model.ServerKind {
	switch s {
	case "Standalone":
		return model.Standalone
	case "RSOther":
		return model.RSMember
	case "RSPrimary":
		return model.RSPrimary
	case "RSSecondary":
		return model.RSSecondary
	case "RSArbiter":
		return model.RSArbiter
	case "RSGhost":
		return model.RSGhost
	case "Mongos":
		return model.Mongos
	}

	return model.Unknown
}

func findServerByAddress(servers []*model.Server, address string) *model.Server {
	for _, server := range servers {
		if server.Addr.String() == address {
			return server
		}
	}

	return nil
}

func anyTagsInSets(sets []model.TagSet) bool {
	for _, set := range sets {
		if len(set) > 0 {
			return true
		}
	}

	return false
}

func compareServers(t *testing.T, expected []*serverDesc, actual []*model.Server) {
	require.Equal(t, len(expected), len(actual))

	for _, expectedServer := range expected {
		actualServer := findServerByAddress(actual, expectedServer.Address)
		require.NotNil(t, actualServer)

		if expectedServer.AverageRTTMS != nil {
			require.Equal(t, *expectedServer.AverageRTTMS, int(actualServer.AverageRTT/time.Millisecond))
		}

		require.Equal(t, expectedServer.Type, actualServer.Kind.String())

		require.Equal(t, len(expectedServer.Tags), len(actualServer.Tags))
		for _, actualTag := range actualServer.Tags {
			expectedTag, ok := expectedServer.Tags[actualTag.Name]
			require.True(t, ok)
			require.Equal(t, expectedTag, actualTag.Value)
		}
	}
}

func selectServers(t *testing.T, test *testCase) error {
	servers := make([]*model.Server, 0, len(test.TopologyDescription.Servers))

	// Times in the JSON files are given as offsets from an unspecified time, but the driver
	// stores the lastWrite field as a timestamp, so we arbitrarily choose the current time
	// as the base to offset from.
	baseTime := time.Now()

	for _, serverDescription := range test.TopologyDescription.Servers {
		server := &model.Server{
			Addr: model.Addr(serverDescription.Address),
			Kind: serverKindFromString(serverDescription.Type),
		}

		if serverDescription.AverageRTTMS != nil {
			server.AverageRTT = time.Duration(*serverDescription.AverageRTTMS) * time.Millisecond
			server.AverageRTTSet = true
		}

		if test.HeartbeatFrequencyMS != nil {
			server.HeartbeatInterval = time.Duration(*test.HeartbeatFrequencyMS) * time.Millisecond
		}

		if serverDescription.LastUpdateTime != nil {
			ms := int64(*serverDescription.LastUpdateTime)
			server.LastUpdateTime = time.Unix(ms/1e3, ms%1e3/1e6)
		}

		if serverDescription.LastWrite != nil {
			i, err := strconv.ParseInt(serverDescription.LastWrite.LastWriteDate.Value, 10, 64)

			if err != nil {
				return err
			}

			timeWithOffset := baseTime.Add(time.Duration(i) * time.Millisecond)
			server.LastWriteTime = timeWithOffset
		}

		if serverDescription.MaxWireVersion != nil {
			versionRange := model.NewRange(0, *serverDescription.MaxWireVersion)
			server.WireVersion = &versionRange
		}

		if serverDescription.Tags != nil {
			server.Tags = model.NewTagSetFromMap(serverDescription.Tags)
		}

		// Max staleness can't be sent to servers older than 3.4.
		if test.ReadPreference.MaxStaleness != nil {
			server.Version = model.Version{Desc: "3.4.0", Parts: []uint8{3, 4, 0}}
		}

		servers = append(servers, server)
	}

	c := &model.Cluster{
		Kind:    clusterKindFromString(test.TopologyDescription.Type),
		Servers: servers,
	}

	if len(test.ReadPreference.Mode) == 0 {
		test.ReadPreference.Mode = "Primary"
	}

	readprefMode, err := readpref.ModeFromString(test.ReadPreference.Mode)
	if err != nil {
		return err
	}

	options := make([]readpref.Option, 0, 1)

	tagSets := model.NewTagSetsFromMaps(test.ReadPreference.TagSets)
	if anyTagsInSets(tagSets) {
		options = append(options, readpref.WithTagSets(tagSets...))
	}

	if test.ReadPreference.MaxStaleness != nil {
		s := time.Duration(*test.ReadPreference.MaxStaleness) * time.Second
		options = append(options, readpref.WithMaxStaleness(s))
	}

	rp, err := readpref.New(readprefMode, options...)
	if err != nil {
		return err
	}

	selector := readpref.Selector(rp)
	if test.Operation == "write" {
		selector = cluster.CompositeSelector(
			[]cluster.ServerSelector{cluster.WriteSelector(), selector},
		)
	}

	result, err := selector(c, c.Servers)
	if err != nil {
		return err
	}

	compareServers(t, test.SuitableServers, result)

	latencySelector := cluster.LatencySelector(time.Duration(15) * time.Millisecond)
	selector = cluster.CompositeSelector(
		[]cluster.ServerSelector{selector, latencySelector},
	)

	result, err = selector(c, c.Servers)
	if err != nil {
		return err
	}

	compareServers(t, test.InLatencyWindow, result)

	return nil
}

func runTest(t *testing.T, testsDir string, directory string, filename string) {
	filepath := path.Join(testsDir, directory, filename)
	content, err := ioutil.ReadFile(filepath)
	require.NoError(t, err)

	// Remove ".json" from filename.
	filename = filename[:len(filename)-5]
	testName := directory + "/" + filename + ":"

	t.Run(testName, func(t *testing.T) {
		var test testCase
		require.NoError(t, json.Unmarshal(content, &test))

		err := selectServers(t, &test)

		if test.Error == nil || !*test.Error {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	})
}
