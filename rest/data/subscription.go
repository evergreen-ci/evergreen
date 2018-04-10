package data

import (
	"sync"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
)

type DBSubscriptionConnector struct{}

func (dc *DBSubscriptionConnector) SaveSubscriptions(subscriptions []event.Subscription) error {
	catcher := grip.NewSimpleCatcher()
	for _, subscription := range subscriptions {
		catcher.Add(subscription.Upsert())
	}
	return catcher.Resolve()
}

type MockSubscriptionConnector struct {
	mu                sync.RWMutex
	MockSubscriptions []event.Subscription
}

func (mc *MockSubscriptionConnector) SaveSubscriptions(subscriptions []event.Subscription) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.MockSubscriptions = append(mc.MockSubscriptions, subscriptions...)
	return nil
}
