package apm

// MonitorConfig makes it possible to configure the behavior of the
// Monitor. In most cases you can pass a nil value.
type MonitorConfig struct {
	// PopulateEvents, when true, forces the collector to
	// preallocate the output structures, for all filtered
	// combinations of databases, collections, and commands as
	// well as namespaces and commands. This is primarily useful
	// when the output requires a predetermined schema or
	// structure and you want to maintain the shape of the data
	// even if no events for a particular command or namespace
	// occurred.
	//
	// When pre-populating events, you should specify commands to
	// filter and either a list of collections and databases or a
	// list of namespaces.
	PopulateEvents bool

	// Commands, Databases, and Collections give you the ability
	// to whitelist a number of operations to filter and exclude
	// other non-matching operations.
	Commands    []string
	Databases   []string
	Collections []string

	// Tags are added to operations via contexts and permit
	// more granular annotations. You must specify a tag in Tags
	// to tracked counters. If AllTags is set, all tags are
	// tracked and reported.
	Tags    []string
	AllTags bool

	// Namespaces allow you to declare a specific database and
	// collection name as a pair. When specified, only events that
	// match the namespace will be collected. Namespace filtering
	// occurs after the other filtering, including by command.
	Namespaces []Namespace
}

// Namespace defines a MongoDB collection and database.
type Namespace struct {
	DB         string
	Collection string
}

func stringSliceContains(slice []string, item string) bool {
	for idx := range slice {
		if slice[idx] == item {
			return true
		}
	}

	return false
}
func (c *MonitorConfig) shouldTrack(e eventKey) bool {
	if c == nil {
		return true
	}

	if len(c.Databases) > 0 && !stringSliceContains(c.Databases, e.dbName) {
		return false
	}

	if len(c.Collections) > 0 && !stringSliceContains(c.Collections, e.collName) {
		return false
	}

	if len(c.Commands) > 0 && !stringSliceContains(c.Commands, e.cmdName) {
		return false
	}

	if len(c.Namespaces) > 0 {
		for _, ns := range c.Namespaces {
			if ns.DB == e.dbName && ns.Collection == e.collName {
				return true
			}
		}

		return false
	}

	return true
}

func (c *MonitorConfig) window() map[eventKey]*eventRecord {
	out := make(map[eventKey]*eventRecord)
	if c == nil {
		return out
	}

	if !c.PopulateEvents {
		return out
	}

	for _, db := range c.Databases {
		for _, coll := range c.Collections {
			for _, cmd := range c.Commands {
				out[eventKey{dbName: db, collName: coll, cmdName: cmd}] = &eventRecord{Tags: map[string]int64{}}
			}
		}
	}

	for _, ns := range c.Namespaces {
		for _, cmd := range c.Commands {
			out[eventKey{dbName: ns.DB, collName: ns.Collection, cmdName: cmd}] = &eventRecord{Tags: map[string]int64{}}
		}
	}

	return out
}
