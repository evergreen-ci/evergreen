package model

import (
	"10gen.com/mci/db"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

const ArtifactFilesCollection = "artifact_files"

// ArtifactFileEntry stores groups of names and links (not content!) for
// files uploaded to the api server by a running agent. These links could
// be for build or task-relevant files (things like extra results,
// test coverage, etc.)
type ArtifactFileEntry struct {
	TaskId          string         `json:"task" bson:"task"`
	TaskDisplayName string         `json:"task_name" bson:"task_name"`
	BuildId         string         `json:"build" bson:"build"`
	Files           []ArtifactFile `json:"files" bson:"files"`
}

// ArtifactFileParams stores file entries as key-value pairs, for easy
// parameter parsing.
// Key = Human-readable name for file
// Value = link for the file
type ArtifactFileParams map[string]string

// ArtifactFile is a pairing of name and link for easy storage/display
type ArtifactFile struct {
	// Name is a human-readable name for the file being linked, e.g. "Coverage Report"
	Name string `json:"name" bson:"name"`
	// Link is the link to the file, e.g. "http://fileserver/coverage.html"
	Link string `json:"link" bson:"link"`
}

var (
	// bson fields for the task struct
	ArtifactFileTaskIdKey   = MustHaveBsonTag(ArtifactFileEntry{}, "TaskId")
	ArtifactFileTaskNameKey = MustHaveBsonTag(ArtifactFileEntry{}, "TaskDisplayName")
	ArtifactFileBuildIdKey  = MustHaveBsonTag(ArtifactFileEntry{}, "BuildId")
	ArtifactFileFilesKey    = MustHaveBsonTag(ArtifactFileEntry{}, "Files")
	ArtifactFileNameKey     = MustHaveBsonTag(ArtifactFile{}, "Name")
	ArtifactFileLinkKey     = MustHaveBsonTag(ArtifactFile{}, "Link")
)

// this function turns the parameter map into an array of
// ArtifactFile structs
func (params ArtifactFileParams) Array() []ArtifactFile {
	var filesArray []ArtifactFile
	for name, link := range params {
		filesArray = append(filesArray, ArtifactFile{name, link})
	}
	return filesArray
}

//====== DB Stuff ======

func FindOneArtifactFileEntryByTask(taskId string) (*ArtifactFileEntry, error) {
	entry := ArtifactFileEntry{}
	err := db.FindOne(
		ArtifactFilesCollection,
		bson.M{ArtifactFileTaskIdKey: taskId},
		bson.M{},
		[]string{},
		&entry,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return &entry, err
}

func FindArtifactFilesForTask(taskId string) ([]ArtifactFile, error) {
	entry, err := FindOneArtifactFileEntryByTask(taskId)
	if entry == nil {
		return []ArtifactFile{}, err
	}
	return entry.Files, err
}

// FindArtifactFileEntriesForBuild returns all of the task artifacts with a given
// buildId. Useful for aggregating task files in the UI
func FindArtifactFileEntriesForBuild(buildId string) ([]ArtifactFileEntry, error) {
	entries := []ArtifactFileEntry{}
	err := db.FindAll(
		ArtifactFilesCollection,
		bson.M{ArtifactFileBuildIdKey: buildId},
		db.NoProjection,
		[]string{ArtifactFileTaskNameKey},
		db.NoSkip,
		db.NoLimit,
		&entries,
	)
	return entries, err
}

// Upsert updates the files entry in the db if an entry already exists,
// overwriting the existing file data. If no entry exists, one is created
func (entry ArtifactFileEntry) Upsert() error {
	for _, file := range entry.Files {
		_, err := db.Upsert(
			ArtifactFilesCollection,
			bson.M{
				ArtifactFileTaskIdKey:   entry.TaskId,
				ArtifactFileTaskNameKey: entry.TaskDisplayName,
				ArtifactFileBuildIdKey:  entry.BuildId,
			},
			bson.M{
				"$addToSet": bson.M{
					ArtifactFileFilesKey: file,
				},
			},
		)
		if err != nil {
			return err
		}
	}
	return nil
}
