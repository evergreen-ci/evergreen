// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package session

import (
	"testing"

	"github.com/mongodb/mongo-go-driver/bson/primitive"
	"github.com/mongodb/mongo-go-driver/internal/testutil/helpers"
	"github.com/mongodb/mongo-go-driver/x/bsonx"
	"github.com/mongodb/mongo-go-driver/x/mongo/driver/uuid"
	"github.com/stretchr/testify/require"
)

var consistent = true
var sessionOpts = &ClientOptions{
	CausalConsistency: &consistent,
}

func compareOperationTimes(t *testing.T, expected *primitive.Timestamp, actual *primitive.Timestamp) {
	if expected.T != actual.T {
		t.Fatalf("T value mismatch; expected %d got %d", expected.T, actual.T)
	}

	if expected.I != actual.I {
		t.Fatalf("I value mismatch; expected %d got %d", expected.I, actual.I)
	}
}

func TestClientSession(t *testing.T) {
	var clusterTime1 = bsonx.Doc{{"$clusterTime",
		bsonx.Document(bsonx.Doc{{"clusterTime", bsonx.Timestamp(10, 5)}})}}
	var clusterTime2 = bsonx.Doc{{"$clusterTime",
		bsonx.Document(bsonx.Doc{{"clusterTime", bsonx.Timestamp(5, 5)}})}}
	var clusterTime3 = bsonx.Doc{{"$clusterTime",
		bsonx.Document(bsonx.Doc{{"clusterTime", bsonx.Timestamp(5, 0)}})}}

	t.Run("TestMaxClusterTime", func(t *testing.T) {
		maxTime := MaxClusterTime(clusterTime1, clusterTime2)
		if !maxTime.Equal(clusterTime1) {
			t.Errorf("Wrong max time")
		}

		maxTime = MaxClusterTime(clusterTime3, clusterTime2)
		if !maxTime.Equal(clusterTime2) {
			t.Errorf("Wrong max time")
		}
	})

	t.Run("TestAdvanceClusterTime", func(t *testing.T) {
		id, _ := uuid.New()
		sess, err := NewClientSession(&Pool{}, id, Explicit, sessionOpts)
		require.Nil(t, err, "Unexpected error")
		err = sess.AdvanceClusterTime(clusterTime2)
		require.Nil(t, err, "Unexpected error")
		if !sess.ClusterTime.Equal(clusterTime2) {
			t.Errorf("Session cluster time incorrect, expected %v, received %v", clusterTime2, sess.ClusterTime)
		}
		err = sess.AdvanceClusterTime(clusterTime3)
		require.Nil(t, err, "Unexpected error")
		if !sess.ClusterTime.Equal(clusterTime2) {
			t.Errorf("Session cluster time incorrect, expected %v, received %v", clusterTime2, sess.ClusterTime)
		}
		err = sess.AdvanceClusterTime(clusterTime1)
		require.Nil(t, err, "Unexpected error")
		if !sess.ClusterTime.Equal(clusterTime1) {
			t.Errorf("Session cluster time incorrect, expected %v, received %v", clusterTime1, sess.ClusterTime)
		}
		sess.EndSession()
	})

	t.Run("TestEndSession", func(t *testing.T) {
		id, _ := uuid.New()
		sess, err := NewClientSession(&Pool{}, id, Explicit, sessionOpts)
		require.Nil(t, err, "Unexpected error")
		sess.EndSession()
		err = sess.UpdateUseTime()
		require.NotNil(t, err, "Expected error, received nil")
	})

	t.Run("TestAdvanceOperationTime", func(t *testing.T) {
		id, _ := uuid.New()
		sess, err := NewClientSession(&Pool{}, id, Explicit, sessionOpts)
		require.Nil(t, err, "Unexpected error")

		optime1 := &primitive.Timestamp{
			T: 1,
			I: 0,
		}
		err = sess.AdvanceOperationTime(optime1)
		testhelpers.RequireNil(t, err, "error updating first operation time: %s", err)
		compareOperationTimes(t, optime1, sess.OperationTime)

		optime2 := &primitive.Timestamp{
			T: 2,
			I: 0,
		}
		err = sess.AdvanceOperationTime(optime2)
		testhelpers.RequireNil(t, err, "error updating second operation time: %s", err)
		compareOperationTimes(t, optime2, sess.OperationTime)

		optime3 := &primitive.Timestamp{
			T: 2,
			I: 1,
		}
		err = sess.AdvanceOperationTime(optime3)
		testhelpers.RequireNil(t, err, "error updating third operation time: %s", err)
		compareOperationTimes(t, optime3, sess.OperationTime)

		err = sess.AdvanceOperationTime(&primitive.Timestamp{
			T: 1,
			I: 10,
		})
		testhelpers.RequireNil(t, err, "error updating fourth operation time: %s", err)
		compareOperationTimes(t, optime3, sess.OperationTime)
		sess.EndSession()
	})

	t.Run("TestTransactionState", func(t *testing.T) {
		id, _ := uuid.New()
		sess, err := NewClientSession(&Pool{}, id, Explicit, nil)
		require.Nil(t, err, "Unexpected error")

		err = sess.CommitTransaction()
		if err != ErrNoTransactStarted {
			t.Errorf("expected error, got %v", err)
		}

		err = sess.AbortTransaction()
		if err != ErrNoTransactStarted {
			t.Errorf("expected error, got %v", err)
		}

		if sess.state != None {
			t.Errorf("incorrect session state, expected None, received %v", sess.state)
		}

		err = sess.StartTransaction(nil)
		require.Nil(t, err, "error starting transaction: %s", err)
		if sess.state != Starting {
			t.Errorf("incorrect session state, expected Starting, received %v", sess.state)
		}

		err = sess.StartTransaction(nil)
		if err != ErrTransactInProgress {
			t.Errorf("expected error, got %v", err)
		}

		sess.ApplyCommand()
		if sess.state != InProgress {
			t.Errorf("incorrect session state, expected InProgress, received %v", sess.state)
		}

		err = sess.StartTransaction(nil)
		if err != ErrTransactInProgress {
			t.Errorf("expected error, got %v", err)
		}

		err = sess.CommitTransaction()
		require.Nil(t, err, "error committing transaction: %s", err)
		if sess.state != Committed {
			t.Errorf("incorrect session state, expected Committed, received %v", sess.state)
		}

		err = sess.AbortTransaction()
		if err != ErrAbortAfterCommit {
			t.Errorf("expected error, got %v", err)
		}

		err = sess.StartTransaction(nil)
		require.Nil(t, err, "error starting transaction: %s", err)
		if sess.state != Starting {
			t.Errorf("incorrect session state, expected Starting, received %v", sess.state)
		}

		err = sess.AbortTransaction()
		require.Nil(t, err, "error aborting transaction: %s", err)
		if sess.state != Aborted {
			t.Errorf("incorrect session state, expected Aborted, received %v", sess.state)
		}

		err = sess.AbortTransaction()
		if err != ErrAbortTwice {
			t.Errorf("expected error, got %v", err)
		}

		err = sess.CommitTransaction()
		if err != ErrCommitAfterAbort {
			t.Errorf("expected error, got %v", err)
		}
	})
}
