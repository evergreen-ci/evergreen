package event

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type eventSuite struct {
	suite.Suite
}

func TestEventSuite(t *testing.T) {
	suite.Run(t, &eventSuite{})
}

func (s *eventSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *eventSuite) SetupTest() {
	s.NoError(db.ClearCollections(AllLogCollection, TaskLogCollection))
}

func (s *eventSuite) TestMarshallAndUnarshallingStructsHaveSameTags() {
	realt := reflect.TypeOf(&EventLogEntry{}).Elem()
	tempt := reflect.TypeOf(&unmarshalEventLogEntry{}).Elem()

	s.Require().Equal(realt.NumField(), tempt.NumField())
	for i := 0; i < realt.NumField(); i++ {
		s.True(reflect.DeepEqual(realt.Field(i).Tag, tempt.Field(i).Tag))
	}
}

func (s *eventSuite) TestTerribleUnmarshallerWithOldResourceType() {
	logger := NewDBEventLogger(AllLogCollection)
	for k, v := range eventRegistry {
		event := EventLogEntry{
			ResourceId: "TEST1",
			EventType:  "TEST2",
			Timestamp:  time.Now().Round(time.Millisecond).Truncate(time.Millisecond),
			Data:       v(),
		}
		found, rTypeTag := findResourceTypeIn(event.Data)
		s.True(found)
		s.Equal(k, rTypeTag)
		if e, ok := event.Data.(*rawAdminEventData); ok {
			// sad violin is sad. bson.Raw cannot be empty, so
			// we set the Kind to 0x0A, which is a BSON null
			e.Changes.Before = bson.Raw{
				Kind: 0x0A,
			}
			e.Changes.After = bson.Raw{
				Kind: 0x0A,
			}
		}

		s.Equal("TEST1", event.ResourceId)
		s.Equal("TEST2", event.EventType)
		s.NoError(logger.LogEvent(&event))
		fetchedEvents, err := Find(AllLogCollection, db.Query(bson.M{}))
		s.NoError(err)
		s.Require().Len(fetchedEvents, 1)
		s.True(fetchedEvents[0].Data.IsValid())
		s.IsType(v(), fetchedEvents[0].Data)
		s.NotNil(fetchedEvents[0].Data)
		s.NoError(db.ClearCollections(AllLogCollection))
	}
}

const expectedJSON1 = `{"processed_at":"2017-06-20T18:07:24.991Z","timestamp":"2017-06-20T18:07:24.991Z","resource_id":"macos.example.com","event_type":"HOST_TASK_FINISHED","data":{"resource_type":"HOST","task_id":"mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44","task_status":"success","successful":false,"duration":0}}`
const expectedJSON2 = `{"resource_type":"HOST","processed_at":"2017-06-20T18:07:24.991Z","timestamp":"2017-06-20T18:07:24.991Z","resource_id":"macos.example.com","event_type":"HOST_TASK_FINISHED","data":{"task_id":"mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44","task_status":"success","successful":false,"duration":0}}`
const expectedJSON3 = `{"resource_type":"HOST","processed_at":"2017-06-20T18:07:24.991Z","timestamp":"2017-06-20T18:07:24.991Z","resource_id":"macos.example.com","event_type":"HOST_TASK_FINISHED","data":{"resource_type":"HOST","task_id":"mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44","task_status":"success","successful":false,"duration":0}}`

func (s *eventSuite) checkRealData(e *EventLogEntry, loc *time.Location) {
	s.NotPanics(func() {
		s.Equal("HOST_TASK_FINISHED", e.EventType)
		s.Equal("macos.example.com", e.ResourceId)

		eventData, ok := e.Data.(*HostEventData)
		s.True(ok)

		//s.Equal("HOST", eventData.ResourceType)
		s.Equal("mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44", eventData.TaskId)
		s.Equal("success", eventData.TaskStatus)
	})
}

func (s *eventSuite) TestWithRealData() {
	loc, err := time.LoadLocation("UTC")
	s.NoError(err)
	date, err := time.ParseInLocation(time.RFC3339Nano, "2017-06-20T18:07:24.991Z", loc)
	s.NoError(err)
	s.Equal("2017-06-20T18:07:24.991Z", date.Format(time.RFC3339Nano))

	// unmarshaller works with r_type in the embedded document set
	data := bson.M{
		idKey:          bson.ObjectIdHex("5949645c9acd9604fdd202d7"),
		TimestampKey:   date,
		ResourceIdKey:  "macos.example.com",
		TypeKey:        "HOST_TASK_FINISHED",
		processedAtKey: date,
		DataKey: bson.M{
			ResourceTypeKey:   "HOST",
			"t_id":            "mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44",
			hostDataStatusKey: "success",
		},
	}
	s.NoError(db.Insert(AllLogCollection, data))
	entries, err := Find(AllLogCollection, db.Query(bson.M{idKey: bson.ObjectIdHex("5949645c9acd9604fdd202d7")}))
	s.NoError(err)
	s.Len(entries, 1)
	s.NotPanics(func() {
		s.checkRealData(&entries[0], loc)
		// Verify that JSON unmarshals as expected
		entries[0].Timestamp = entries[0].Timestamp.In(loc)
		entries[0].ProcessedAt = entries[0].ProcessedAt.In(loc)
		s.Empty(entries[0].ResourceType)

		found, tag := findResourceTypeIn(entries[0].Data)
		s.True(found)
		s.Equal("HOST", tag)

		var bytes []byte
		bytes, err = json.Marshal(entries[0])
		s.NoError(err)
		s.Equal(expectedJSON1, string(bytes))
	})

	// unmarshaller works with r_type in the root document set
	data = bson.M{
		idKey:           bson.ObjectIdHex("5949645c9acd9604fdd202d8"),
		TimestampKey:    date,
		ResourceIdKey:   "macos.example.com",
		TypeKey:         "HOST_TASK_FINISHED",
		ResourceTypeKey: "HOST",
		processedAtKey:  date,
		DataKey: bson.M{
			"t_id":            "mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44",
			hostDataStatusKey: "success",
		},
	}
	s.NoError(db.Insert(AllLogCollection, data))
	entries, err = Find(AllLogCollection, db.Query(bson.M{idKey: bson.ObjectIdHex("5949645c9acd9604fdd202d8")}))
	s.NoError(err)
	s.Len(entries, 1)
	s.NotPanics(func() {
		s.checkRealData(&entries[0], loc)
		entries[0].Timestamp = entries[0].Timestamp.In(loc)
		entries[0].ProcessedAt = entries[0].ProcessedAt.In(loc)
		s.Equal("HOST", entries[0].ResourceType)

		found, tag := findResourceTypeIn(entries[0].Data)
		s.True(found)
		s.Empty(tag)

		var bytes []byte
		bytes, err := json.Marshal(entries[0])
		s.NoError(err)
		s.Equal(expectedJSON2, string(bytes))
	})

	// unmarshaller works with both r_type fields set
	data = bson.M{
		idKey:           bson.ObjectIdHex("5949645c9acd9604fdd202d9"),
		TimestampKey:    date,
		ResourceIdKey:   "macos.example.com",
		TypeKey:         "HOST_TASK_FINISHED",
		ResourceTypeKey: "HOST",
		processedAtKey:  date,
		DataKey: bson.M{
			ResourceTypeKey:   "HOST",
			"t_id":            "mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44",
			hostDataStatusKey: "success",
		},
	}
	s.NoError(db.Insert(AllLogCollection, data))
	entries, err = Find(AllLogCollection, db.Query(bson.M{idKey: bson.ObjectIdHex("5949645c9acd9604fdd202d9")}))
	s.NoError(err)
	s.Len(entries, 1)
	s.NotPanics(func() {
		s.checkRealData(&entries[0], loc)
		entries[0].Timestamp = entries[0].Timestamp.In(loc)
		entries[0].ProcessedAt = entries[0].ProcessedAt.In(loc)
		s.Equal("HOST", entries[0].ResourceType)

		found, tag := findResourceTypeIn(entries[0].Data)
		s.True(found)
		s.Equal("HOST", tag)

		var bytes []byte
		bytes, err := json.Marshal(entries[0])
		s.NoError(err)
		s.Equal(expectedJSON3, string(bytes))
	})
}

func (s *eventSuite) TestEventWithNilData() {
	logger := NewDBEventLogger(AllLogCollection)
	event := EventLogEntry{
		ID:         bson.NewObjectId(),
		ResourceId: "TEST1",
		EventType:  "TEST2",
		Timestamp:  time.Now().Round(time.Millisecond).Truncate(time.Millisecond),
	}
	s.Errorf(logger.LogEvent(&event), "event log entry cannot have nil Data")

	s.NotPanics(func() {
		s.NoError(db.Insert(AllLogCollection, event))
		fetchedEvents, err := Find(AllLogCollection, db.Query(bson.M{}))
		s.Error(err)
		s.Empty(fetchedEvents)
	})
}

func (s *eventSuite) TestEventRegistryItemsAreSane() {
	for k, _ := range eventRegistry {
		event := NewEventFromType(k)
		s.NotNil(event)
		found, rTypeTag := findResourceTypeIn(event)

		t := reflect.TypeOf(event)
		s.True(found, `'%s' does not have a bson:"r_type,omitempty" tag`, t.String())
		s.Equal(k, rTypeTag, "'%s''s r_type does not match the registry key", t.String())

		// ensure all fields have bson and json tags
		elem := t.Elem()
		for i := 0; i < elem.NumField(); i++ {
			f := elem.Field(i)
			bsonTag := f.Tag.Get("bson")
			jsonTag := f.Tag.Get("json")
			s.NotEmpty(bsonTag, "struct %s: field '%s' must have bson tag", t.Name(), f.Name)

			if _, ok := event.(*rawAdminEventData); !ok {
				s.NotEmpty(jsonTag, "struct %s: field '%s' must have json tag", t.Name(), f.Name)
				if bsonTag == resourceTypeKey {
					s.Equal("resource_type", jsonTag)
				}
			}
		}
	}
}

func (s *eventSuite) TestFindResourceTypeIn() {
	succeedStruct := struct {
		Type       string `bson:"r_type,omitempty"`
		OtherThing int    `bson:"other"`
	}{Type: "something"}
	failStruct := struct {
		WrongThing string `bson:"type"`
		OtherThing string `bson:"other"`
	}{WrongThing: "wrong"}
	failStruct2 := struct {
		WrongThing int `bson:"r_type"`
		OtherThing int `bson:"other"`
	}{WrongThing: 1}

	found, data := findResourceTypeIn(&succeedStruct)
	s.True(found)
	s.Equal("something", data)

	found, data = findResourceTypeIn(&failStruct)
	s.False(found)
	s.Empty(data)

	found, data = findResourceTypeIn(&failStruct2)
	s.False(found)
	s.Empty(data)

	found, data = findResourceTypeIn(nil)
	s.False(found)
	s.Empty(data)
}

func (s *eventSuite) TestMarkProcessed() {
	startTime := time.Now()

	logger := NewDBEventLogger(AllLogCollection)
	event := EventLogEntry{
		ResourceId: "TEST1",
		EventType:  "TEST2",
		Timestamp:  time.Now().Round(time.Millisecond).Truncate(time.Millisecond),
		Data:       NewEventFromType(ResourceTypeHost),
	}
	processed, ptime := event.Processed()
	s.False(processed)
	s.Zero(ptime)

	s.EqualError(logger.MarkProcessed(&event), "event has no ID")
	event.ID = bson.NewObjectId()
	s.EqualError(logger.MarkProcessed(&event), "failed to update process time: not found")

	s.NoError(logger.LogEvent(&event))

	var fetchedEvent EventLogEntry
	err := db.FindOneQ(AllLogCollection, db.Q{}, &fetchedEvent)
	s.NoError(err)

	processed, ptime = fetchedEvent.Processed()
	s.False(processed)
	s.Zero(ptime)

	time.Sleep(time.Millisecond)
	s.NoError(logger.MarkProcessed(&fetchedEvent))
	processed, ptime = fetchedEvent.Processed()
	s.True(processed)
	s.True(ptime.After(startTime))
}

func (s *eventSuite) TestQueryWithBothRTypes() {
	data := []bson.M{
		{
			resourceTypeKey: "test",
			DataKey:         bson.M{},
		},
		{
			DataKey: bson.M{
				resourceTypeKey: "test",
			},
		},
		{
			resourceTypeKey: "test",
			DataKey: bson.M{
				resourceTypeKey: "somethingelse",
			},
		},
	}

	for i := range data {
		s.NoError(db.Insert(AllLogCollection, data[i]))
	}

	out := []bson.M{}
	s.NoError(db.FindAllQ(AllLogCollection, db.Query(eitherResourceTypeKeyIs("test")), &out))

	s.Len(out, 3)
}

func (s *eventSuite) TestFindUnprocessedEvents() {
	data := []bson.M{
		{
			DataKey: bson.M{
				resourceTypeKey: ResourceTypeHost,
			},
		},
		{
			processedAtKey: time.Time{},
			DataKey: bson.M{
				resourceTypeKey: ResourceTypeHost,
			},
		},
		{
			processedAtKey: time.Now(),
			DataKey: bson.M{
				resourceTypeKey: ResourceTypeHost,
			},
		},
	}
	for i := range data {
		s.NoError(db.Insert(AllLogCollection, data[i]))
	}
	events, err := Find(AllLogCollection, UnprocessedEvents())
	s.NoError(err)
	s.Len(events, 1)

	s.NotPanics(func() {
		processed, ptime := events[0].Processed()
		s.Zero(ptime)
		s.False(processed)
	})
}
