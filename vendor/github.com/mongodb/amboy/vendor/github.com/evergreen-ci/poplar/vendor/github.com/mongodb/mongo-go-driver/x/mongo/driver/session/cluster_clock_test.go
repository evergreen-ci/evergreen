// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package session

import (
	"testing"

	"github.com/mongodb/mongo-go-driver/x/bsonx"
)

func TestClusterClock(t *testing.T) {
	var clusterTime1 = bsonx.Doc{{"$clusterTime",
		bsonx.Document(bsonx.Doc{{"clusterTime", bsonx.Timestamp(10, 5)}})}}
	var clusterTime2 = bsonx.Doc{{"$clusterTime",
		bsonx.Document(bsonx.Doc{{"clusterTime", bsonx.Timestamp(5, 5)}})}}
	var clusterTime3 = bsonx.Doc{{"$clusterTime",
		bsonx.Document(bsonx.Doc{{"clusterTime", bsonx.Timestamp(5, 0)}})}}

	t.Run("ClusterTime", func(t *testing.T) {
		clock := ClusterClock{}
		clock.AdvanceClusterTime(clusterTime3)
		done := make(chan struct{})
		go func() {
			clock.AdvanceClusterTime(clusterTime1)
			done <- struct{}{}
		}()
		clock.AdvanceClusterTime(clusterTime2)

		<-done
		if !clock.GetClusterTime().Equal(clusterTime1) {
			t.Errorf("Expected cluster time %v, received %v", clusterTime1, clock.GetClusterTime())
		}
	})
}
