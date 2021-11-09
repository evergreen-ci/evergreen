package hdrhist

import (
	"encoding/json"

	"github.com/evergreen-ci/birch"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// A Snapshot is an exported view of a Histogram, useful for serializing them.
// A Histogram can be constructed from it by passing it to Import.
type Snapshot struct {
	LowestTrackableValue  int64   `bson:"lowest" json:"lowest" yaml:"lowest"`
	HighestTrackableValue int64   `bson:"highest" json:"highest" yaml:"highest"`
	SignificantFigures    int64   `bson:"figures" json:"figures" yaml:"figures"`
	Counts                []int64 `bson:"counts" json:"counts" yaml:"counts"`
}

func (h *Histogram) MarshalDocument() (*birch.Document, error) {
	return birch.DC.Make(5).Append(
		birch.EC.Int64("lowest", h.lowestTrackableValue),
		birch.EC.Int64("highest", h.highestTrackableValue),
		birch.EC.Int64("figures", h.significantFigures),
		birch.EC.SliceInt64("counts", h.counts),
	), nil
}

func (h *Histogram) MarshalBSON() ([]byte, error) { return birch.MarshalDocumentBSON(h) }
func (h *Histogram) MarshalJSON() ([]byte, error) { return json.Marshal(h.Export()) }

func (h *Histogram) UnmarshalBSON(in []byte) error {
	s := &Snapshot{}
	if err := bson.Unmarshal(in, s); err != nil {
		return errors.WithStack(err)
	}

	*h = *Import(s)
	return nil
}

func (h *Histogram) UnmarshalJSON(in []byte) error {
	s := &Snapshot{}
	if err := json.Unmarshal(in, s); err != nil {
		return errors.WithStack(err)
	}

	*h = *Import(s)
	return nil
}
