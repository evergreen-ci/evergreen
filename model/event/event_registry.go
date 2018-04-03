package event

import (
	"fmt"
	"sync"
)

type eventFactory func() interface{}

var eventRegistry map[string]eventFactory = map[string]eventFactory{}
var eventRegistryM = sync.RWMutex{}

var eventSubscribabilityRegistry map[EventLogEntry]bool = map[EventLogEntry]bool{}
var eventSubscribabilityRegistryM = sync.RWMutex{}

// registryAdd adds an event data factory to the registry with the given resource
// type. registryAdd will panic if you attempt to add a resourceType more than once
func registryAdd(resourceType string, f eventFactory) {
	eventRegistryM.Lock()
	defer eventRegistryM.Unlock()

	_, ok := eventRegistry[resourceType]
	if ok {
		panic(fmt.Sprintf("attempted to register event '%s' more than once", resourceType))
	}

	eventRegistry[resourceType] = f
}

// allowSubscriptions marks a combination of resource type and Event Type as
// subscribable. Events marked subscribable will be saved with an empty
// processed_at time, so they can be picked up by the event driven notifications
// amboy jobs. Events not explicitly marked, will have their processed_at times
// set to notSubscribableTimeString
// allowSubscriptions will panic if you try to mark a pair
// (resourceType, eventType) as subscribable more than once.
func allowSubscriptions(resourceType, eventType string) {
	eventSubscribabilityRegistryM.Lock()
	defer eventSubscribabilityRegistryM.Unlock()

	e := EventLogEntry{
		ResourceType: resourceType,
		EventType:    eventType,
	}
	_, ok := eventSubscribabilityRegistry[e]
	if ok {
		panic(fmt.Sprintf("attempted to enable subscribability for event '%s/%s' more than once", resourceType, eventType))
	}

	eventSubscribabilityRegistry[e] = true
}

// isSubscribable looks to see if a (resourceType, eventType) pair is allowed
// to be subscribed to
func isSubscribable(resourceType, eventType string) bool {
	eventSubscribabilityRegistryM.RLock()
	defer eventSubscribabilityRegistryM.RUnlock()

	e := EventLogEntry{
		ResourceType: resourceType,
		EventType:    eventType,
	}

	allowSubs, _ := eventSubscribabilityRegistry[e]

	return allowSubs
}

func NewEventFromType(resourceType string) interface{} {
	eventRegistryM.RLock()
	defer eventRegistryM.RUnlock()

	f, ok := eventRegistry[resourceType]
	if !ok {
		return nil
	}

	return f()
}

func taskEventFactory() interface{} {
	return &TaskEventData{}
}

func hostEventFactory() interface{} {
	return &HostEventData{}
}

func distroEventFactory() interface{} {
	return &DistroEventData{}
}

func schedulerEventFactory() interface{} {
	return &SchedulerEventData{}
}

func taskSystemResourceEventFactory() interface{} {
	return &TaskSystemResourceData{}
}

func taskProcessResourceEventFactory() interface{} {
	return &TaskProcessResourceData{}
}

func adminEventFactory() interface{} {
	return &rawAdminEventData{}
}

func testEventFactory() interface{} {
	return &TestEvent{}
}
