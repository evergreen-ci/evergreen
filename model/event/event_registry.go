package event

import (
	"fmt"
	"sync"
)

type eventDataFactory func() interface{}

type eventRegistry struct {
	lock sync.RWMutex

	types          map[string]eventDataFactory
	isSubscribable map[EventLogEntry]bool
}

var registry *eventRegistry

func init() {
	registry = &eventRegistry{
		types:          map[string]eventDataFactory{},
		isSubscribable: map[EventLogEntry]bool{},
	}

	registry.AddType(ResourceTypeAdmin, adminEventDataFactory)
	registry.AddType(ResourceTypeBuild, buildEventDataFactory)
	registry.AddType(ResourceTypeDistro, distroEventDataFactory)
	registry.AddType(ResourceTypeUser, userEventDataFactory)
	registry.AllowSubscription(ResourceTypeBuild, BuildStateChange)
	registry.AllowSubscription(ResourceTypeBuild, BuildGithubCheckFinished)

	registry.AddType(ResourceTypeCommitQueue, commitQueueEventDataFactory)
	registry.AllowSubscription(ResourceTypeCommitQueue, CommitQueueStartTest)
	registry.AllowSubscription(ResourceTypeCommitQueue, CommitQueueConcludeTest)
	registry.AllowSubscription(ResourceTypeCommitQueue, CommitQueueEnqueueFailed)
}

// AddType adds an event data factory to the registry with the given resource
// type. AddType will panic if you attempt to add the same resourceType more
// than once
func (r *eventRegistry) AddType(resourceType string, f eventDataFactory) {
	r.lock.Lock()
	defer r.lock.Unlock()

	_, ok := r.types[resourceType]
	if ok {
		panic(fmt.Sprintf("attempted to register event '%s' more than once", resourceType))
	}

	r.types[resourceType] = f
}

// AllowSubscription a combination of resource type and Event Type as
// subscribable. Events marked subscribable will be saved with an empty
// processed_at time, so they can be picked up by the event driven notifications
// amboy jobs. Events not explicitly marked, will have their processed_at times
// set to notSubscribableTimeString
// AllowSubscription will panic if you try to mark a pair
// (resourceType, eventType) as subscribable more than once.
func (r *eventRegistry) AllowSubscription(resourceType, eventType string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	e := EventLogEntry{
		ResourceType: resourceType,
		EventType:    eventType,
	}
	_, ok := r.isSubscribable[e]
	if ok {
		panic(fmt.Sprintf("attempted to enable subscribability for event '%s/%s' more than once", resourceType, eventType))
	}

	r.isSubscribable[e] = true
}

// IsSubscribable looks to see if a (resourceType, eventType) pair is allowed
// to be subscribed to
func (r *eventRegistry) IsSubscribable(resourceType, eventType string) bool {
	r.lock.RLock()
	defer r.lock.RUnlock()

	e := EventLogEntry{
		ResourceType: resourceType,
		EventType:    eventType,
	}

	return r.isSubscribable[e]
}

func NewEventFromType(resourceType string) interface{} {
	registry.lock.RLock()
	defer registry.lock.RUnlock()

	f, ok := registry.types[resourceType]
	if !ok {
		return nil
	}

	return f()
}

func taskEventDataFactory() interface{} {
	return &TaskEventData{}
}

func hostEventDataFactory() interface{} {
	return &HostEventData{}
}

func distroEventDataFactory() interface{} {
	return &DistroEventData{}
}

func schedulerEventDataFactory() interface{} {
	return &SchedulerEventData{}
}

func adminEventDataFactory() interface{} {
	return &rawAdminEventData{}
}

func userEventDataFactory() interface{} {
	return &userData{}
}

func podEventDataFactory() interface{} {
	return &podData{}
}
