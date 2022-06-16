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
	neverExpires   map[EventLogEntry]bool
}

var registry eventRegistry

// AddType adds an event data factory to the registry with the given resource
// type. AddType will panic if you attempt to add the same resourceType more
// than once
func (r *eventRegistry) AddType(resourceType string, f eventDataFactory) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.types == nil {
		r.types = make(map[string]eventDataFactory)
	}

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

	if r.isSubscribable == nil {
		r.isSubscribable = make(map[EventLogEntry]bool)
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

func (r *eventRegistry) setNeverExpire(resourceType, eventType string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.neverExpires == nil {
		r.neverExpires = make(map[EventLogEntry]bool)
	}
	r.neverExpires[EventLogEntry{ResourceType: resourceType, EventType: eventType}] = true
}

func (r *eventRegistry) isExpirable(resourceType, eventType string) bool {
	r.lock.RLock()
	defer r.lock.RUnlock()

	e := EventLogEntry{
		ResourceType: resourceType,
		EventType:    eventType,
	}

	return !r.neverExpires[e]
}

func (r *eventRegistry) newEventFromType(resourceType string) interface{} {
	registry.lock.RLock()
	defer registry.lock.RUnlock()

	f, ok := registry.types[resourceType]
	if !ok {
		return nil
	}

	return f()
}
