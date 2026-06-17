package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/graphql/loaders"
	"github.com/evergreen-ci/evergreen/model/user"
)

// getVersionAuthorDBUser returns a version author as a *user.DBUser. The id, displayName,
// and emailAddress fields are served from the denormalized version document; requesting any
// other field triggers a database fetch.
func getVersionAuthorDBUser(ctx context.Context, authorID, authorName, authorEmail string) (*user.DBUser, error) {
	fromVersion := &user.DBUser{
		Id:           authorID,
		DispName:     authorName,
		EmailAddress: authorEmail,
	}

	requestedFields := graphql.CollectAllFields(ctx)
	needsDBFetch := false
	for _, field := range requestedFields {
		if field != "id" && field != "displayName" && field != "emailAddress" {
			needsDBFetch = true
			break
		}
	}
	if !needsDBFetch {
		return fromVersion, nil
	}

	currentUser := mustHaveUser(ctx)
	if currentUser.Id == authorID {
		return currentUser, nil
	}

	dbUser, err := loaders.GetUser(ctx, authorID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting user '%s': %s", authorID, err.Error()), err)
	}
	// This is most likely a service or reaped user, so just return their info from the version.
	if dbUser == nil {
		return fromVersion, nil
	}

	return dbUser, nil
}
