package event

import (
	"fmt"
	"reflect"
	"sync"
)

type eventFactory func() interface{}

type extraDataKey struct {
	ResourceType string
	Trigger      string
}

type eventRegistry struct {
	lock sync.RWMutex

	types          map[string]eventFactory
	isSubscribable map[EventLogEntry]bool
	extraData      map[extraDataKey]interface{}
}

var registry eventRegistry = eventRegistry{
	types:          map[string]eventFactory{},
	isSubscribable: map[EventLogEntry]bool{},
	extraData:      map[extraDataKey]interface{}{},
}

// AddType adds an event data factory to the registry with the given resource
// type. AddType will panic if you attempt to add the same resourceType more
// than once
func (r *eventRegistry) AddType(resourceType string, f eventFactory) {
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

// RegisterExtraData allows a pair of (resourceType, trigger) to deserialize
// data from the ExtraData field. Once registered, the data is required.
func (r *eventRegistry) RegisterExtraData(resourceType, triggerName string, i interface{}) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if i == nil {
		panic(fmt.Sprintf("attempted to register nil extra data for event '%s', trigger: '%s' more than once", resourceType, triggerName))
	}

	e := extraDataKey{
		ResourceType: resourceType,
		Trigger:      triggerName,
	}

	_, ok := r.extraData[e]
	if ok {
		panic(fmt.Sprintf("attempted to register extra data for event '%s', trigger: '%s' more than once", resourceType, triggerName))
	}

	if reflect.TypeOf(i).Kind() == reflect.Struct {
		panic(fmt.Sprintf("extra data must be a pointer to a struct, saw Kind: %s", reflect.TypeOf(i).Kind().String()))
	}

	r.extraData[e] = i
}

// GetExtraData initializes extra data for the given pair (resourceType, trigger)
// The type returned is guaranteed to be not nil
func (r *eventRegistry) GetExtraData(resourceType, triggerName string) interface{} {
	r.lock.RLock()
	defer r.lock.RUnlock()

	e := extraDataKey{
		ResourceType: resourceType,
		Trigger:      triggerName,
	}

	data, ok := r.extraData[e]
	if !ok {
		return nil
	}

	t := reflect.ValueOf(data).Type()
	return reflect.New(t).Interface()
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
