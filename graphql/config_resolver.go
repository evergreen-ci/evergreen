package graphql

import (
	"context"
	"sort"

	"github.com/evergreen-ci/evergreen/rest/model"
)

// Port is the resolver for the port field.
func (r *containerPoolResolver) Port(ctx context.Context, obj *model.APIContainerPool) (int, error) {
	return int(obj.Port), nil
}

// SecretFields is the resolver for the secretFields field.
func (r *spruceConfigResolver) SecretFields(ctx context.Context, obj *model.APIAdminSettings) ([]string, error) {
	redactedFieldsAsSlice := []string{}
	for field := range redactedFields {
		redactedFieldsAsSlice = append(redactedFieldsAsSlice, field)
	}

	sort.Strings(redactedFieldsAsSlice)

	return redactedFieldsAsSlice, nil
}

// ContainerPool returns ContainerPoolResolver implementation.
func (r *Resolver) ContainerPool() ContainerPoolResolver { return &containerPoolResolver{r} }

// SpruceConfig returns SpruceConfigResolver implementation.
func (r *Resolver) SpruceConfig() SpruceConfigResolver { return &spruceConfigResolver{r} }

type containerPoolResolver struct{ *Resolver }
type spruceConfigResolver struct{ *Resolver }

// !!! WARNING !!!
// The code below was going to be deleted when updating resolvers. It has been copied here so you have
// one last chance to move it out of harms way if you want. There are two reasons this happens:
//  - When renaming or deleting a resolver the old code will be put in here. You can safely delete
//    it when you're done.
//  - You have helper methods in this file. Move them out to keep these resolver files clean.
/*
	func (r *projectTasksPairResolver) AllowedBVs(ctx context.Context, obj *model.APIProjectTasksPair) ([]string, error) {
	panic(fmt.Errorf("not implemented: AllowedBVs - allowedBVs"))
}
func (r *Resolver) ProjectTasksPair() ProjectTasksPairResolver { return &projectTasksPairResolver{r} }
type projectTasksPairResolver struct{ *Resolver }
*/
