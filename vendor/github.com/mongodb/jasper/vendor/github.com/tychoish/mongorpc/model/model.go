package model

import "github.com/tychoish/mongorpc/bson"

type Command struct {
	DB                 string
	Command            string
	Arguments          bson.Simple
	Metadata           bson.Simple
	Inputs             []bson.Simple
	ConvertedFromQuery bool
}

type Delete struct {
	Namespace string
	Filter    bson.Simple
}

type Insert struct {
	Namespace string
	Documents []bson.Simple
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
	Query     bson.Simple
	Project   bson.Simple
}

type Update struct {
	Namespace string
	Filter    bson.Simple
	Update    bson.Simple

	Upsert bool
	Multi  bool
}

type Reply struct {
	Contents       []bson.Simple
	CursorID       int64
	StartingFrom   int32
	CursorNotFound bool
	QueryFailure   bool
}
