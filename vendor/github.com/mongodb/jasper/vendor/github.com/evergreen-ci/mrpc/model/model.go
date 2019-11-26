package model

import "github.com/evergreen-ci/birch"

type Command struct {
	DB                 string
	Command            string
	Arguments          *birch.Document
	Metadata           *birch.Document
	Inputs             []birch.Document
	ConvertedFromQuery bool
}

type Delete struct {
	Namespace string
	Filter    *birch.Document
}

type Insert struct {
	Namespace string
	Documents []birch.Document
}

type GetMore struct {
	Namespace string
	CursorID  int64
	NReturn   int32
}

type Query struct {
	Namespace string
	Skip      int32
	NReturn   int32
	Query     *birch.Document
	Project   *birch.Document
}

type Update struct {
	Namespace string
	Filter    *birch.Document
	Update    *birch.Document

	Upsert bool
	Multi  bool
}

type Reply struct {
	Contents       []birch.Document
	CursorID       int64
	StartingFrom   int32
	CursorNotFound bool
	QueryFailure   bool
}

type Message struct {
	Database   string
	Collection string
	Operation  string
	MoreToCome bool
	Checksum   bool
	Items      []SequenceItem
}

type SequenceItem struct {
	Identifier string
	Documents  []birch.Document
}
