package model

// Configuration describes migrations configured in a file passed to
// anser. This kind of migration are typically populated directly from
// a configuration file, unlike most anser migrations which are
// implemented and described in Go code.
type Configuration struct {
	Options          ApplicationOptions             `bson:"options" json:"options" yaml:"options"`
	SimpleMigrations []ConfigurationSimpleMigration `bson:"simple_migrations" json:"simple_migrations" yaml:"simple_migrations"`
	ManualMigrations []ConfigurationManualMigration `bson:"manual_migrations" json:"manual_migrations" yaml:"manual_migrations"`
	StreamMigrations []ConfigurationManualMigration `bson:"stream_migrations" json:"stream_migrations" yaml:"stream_migrations"`
}

// ApplicationOptions define aspects of the application's behavior as
// a whole.
type ApplicationOptions struct {
	DryRun bool `bson:"dry_run" json:"dry_run" yaml:"dry_run"`
	Limit  int  `bson:"limit" json:"limit" yaml:"limit"`
}

// ConfigurationSimpleMigrations defines a migration that provides, in
// essence a single-document update as the migration.
type ConfigurationSimpleMigration struct {
	Options GeneratorOptions       `bson:"options" json:"options" yaml:"options"`
	Update  map[string]interface{} `bson:"update" json:"update" yaml:"update"`
}

// ConfigurationManualMigrations defines either a stream/manual
// migration, as the definition for either depend on having the
// migration implementation for that name to be defined.
type ConfigurationManualMigration struct {
	Options GeneratorOptions `bson:"options" json:"options" yaml:"options"`
	Name    string           `bson:"name" json:"name" yaml:"name"`
}
