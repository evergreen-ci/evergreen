package event

import (
	"encoding/json"
	"reflect"
	"strings"
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

// resource_type in data should be copied to root document
const expectedJSON1 = `{"resource_type":"HOST","processed_at":"2017-06-20T18:07:24.991Z","timestamp":"2017-06-20T18:07:24.991Z","resource_id":"macos.example.com","event_type":"HOST_TASK_FINISHED","data":{"task_id":"mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44","task_status":"success","successful":false,"duration":0}}`

// resource_type in root document
const expectedJSON2 = `{"resource_type":"HOST","processed_at":"2017-06-20T18:07:24.991Z","timestamp":"2017-06-20T18:07:24.991Z","resource_id":"macos.example.com","event_type":"HOST_TASK_FINISHED","data":{"task_id":"mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44","task_status":"success","successful":false,"duration":0}}`

// resource_type in both
const expectedJSON3 = `{"resource_type":"HOST","processed_at":"2017-06-20T18:07:24.991Z","timestamp":"2017-06-20T18:07:24.991Z","resource_id":"macos.example.com","event_type":"HOST_TASK_FINISHED","data":{"task_id":"mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44","task_status":"success","successful":false,"duration":0}}`

func (s *eventSuite) checkRealData(e *EventLogEntry, loc *time.Location) {
	s.NotPanics(func() {
		s.Equal("HOST_TASK_FINISHED", e.EventType)
		s.Equal("macos.example.com", e.ResourceId)

		eventData, ok := e.Data.(*HostEventData)
		s.True(ok)

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
		idKey:           bson.ObjectIdHex("5949645c9acd9604fdd202d7"),
		TimestampKey:    date,
		ResourceIdKey:   "macos.example.com",
		TypeKey:         "HOST_TASK_FINISHED",
		processedAtKey:  date,
		ResourceTypeKey: "HOST",
		DataKey: bson.M{
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
		s.Equal("HOST", entries[0].ResourceType)
		s.IsType(&HostEventData{}, entries[0].Data)

		found, tag := findResourceTypeIn(entries[0].Data)
		s.False(found)
		s.Empty(tag)

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
		s.IsType(&HostEventData{}, entries[0].Data)

		found, tag := findResourceTypeIn(entries[0].Data)
		s.False(found)
		s.Empty(tag)

		var bytes []byte
		bytes, err = json.Marshal(entries[0])
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
		s.IsType(&HostEventData{}, entries[0].Data)
		s.Equal("HOST", entries[0].ResourceType)

		found, tag := findResourceTypeIn(entries[0].Data)
		s.False(found)
		s.Empty(tag)

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
		s.False(found, `'%s' has a bson:"r_type,omitempty" tag, but should not`, t.String())
		s.Empty(rTypeTag)

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

	s.NotPanics(func() {
		found, data := findResourceTypeIn(succeedStruct)
		s.True(found)
		s.Equal("something", data)

		found, data = findResourceTypeIn(failStruct)
		s.False(found)
		s.Empty(data)

		found, data = findResourceTypeIn(failStruct2)
		s.False(found)
		s.Empty(data)

		found, data = findResourceTypeIn(nil)
		s.False(found)
		s.Empty(data)

		x := 5
		found, data = findResourceTypeIn(x)
		s.False(found)
		s.Empty(data)

		found, data = findResourceTypeIn(&x)
		s.False(found)
		s.Empty(data)
	})
}

func (s *eventSuite) TestMarkProcessed() {
	startTime := time.Now()

	logger := NewDBEventLogger(AllLogCollection)
	event := EventLogEntry{
		ResourceType: ResourceTypeHost,
		ResourceId:   "TEST1",
		EventType:    "TEST2",
		Timestamp:    time.Now().Round(time.Millisecond).Truncate(time.Millisecond),
		Data:         NewEventFromType(ResourceTypeHost),
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

func (s *eventSuite) TestFindUnprocessedEvents() {
	data := []bson.M{
		{
			resourceTypeKey: ResourceTypeHost,
			DataKey:         bson.M{},
		},
		{
			processedAtKey:  time.Time{},
			resourceTypeKey: ResourceTypeHost,
			DataKey:         bson.M{},
		},
		{
			processedAtKey:  time.Now(),
			resourceTypeKey: ResourceTypeHost,
			DataKey:         bson.M{},
		},
	}
	for i := range data {
		s.NoError(db.Insert(AllLogCollection, data[i]))
	}
	events, err := Find(AllLogCollection, db.Query(UnprocessedEvents()))
	s.NoError(err)
	s.Len(events, 1)

	s.NotPanics(func() {
		processed, ptime := events[0].Processed()
		s.Zero(ptime)
		s.False(processed)
	})
}

// findResourceTypeIn attempts to locate a bson tag with "r_type,omitempty" in it.
// If found, this function returns true, and the value of that field
// If not, this function returns false. If it the struct had "r_type" (without
// omitempty), it will return that field's value, otherwise it returns an
// empty string
func findResourceTypeIn(data interface{}) (bool, string) {
	const resourceTypeKeyWithOmitEmpty = resourceTypeKey + ",omitempty"

	if data == nil {
		return false, ""
	}

	v := reflect.ValueOf(data)
	t := reflect.TypeOf(data)

	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false, ""
	}

	for i := 0; i < v.NumField(); i++ {
		fieldType := t.Field(i)
		bsonTag := fieldType.Tag.Get("bson")
		if len(bsonTag) == 0 {
			continue
		}

		if fieldType.Type.Kind() != reflect.String {
			return false, ""
		}

		if bsonTag != resourceTypeKey && !strings.HasPrefix(bsonTag, resourceTypeKey+",") {
			continue
		}

		structData := v.Field(i).String()
		return bsonTag == resourceTypeKeyWithOmitEmpty, structData
	}

	return false, ""
}
