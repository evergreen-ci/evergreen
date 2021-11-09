package model

// MigrationMetadata records data about completed migrations.
type MigrationMetadata struct {
	ID        string `bson:"_id" json:"id" yaml:"id"`
	Migration string `bson:"migration" json:"migration" yaml:"migration"`
	HasErrors bool   `bson:"has_errors" json:"has_errors" yaml:"has_errors"`
	Completed bool   `bson:"completed" json:"completed" yaml:"completed"`
}

// Satisfies reports if a migration has completed without errors.
func (m *MigrationMetadata) Satisfied() bool { return m.Completed && !m.HasErrors }
