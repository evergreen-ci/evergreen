package graphql

import (
	"context"
	"fmt"
	"sort"

	"github.com/evergreen-ci/evergreen/rest/model"
)

// Announcements is the resolver for the announcements field.
func (r *adminSettingsResolver) Announcements(ctx context.Context, obj *model.APIAdminSettings) (*Announcements, error) {
	panic(fmt.Errorf("not implemented: Announcements - announcements"))
}

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

// AdminSettings returns AdminSettingsResolver implementation.
func (r *Resolver) AdminSettings() AdminSettingsResolver { return &adminSettingsResolver{r} }

// ContainerPool returns ContainerPoolResolver implementation.
func (r *Resolver) ContainerPool() ContainerPoolResolver { return &containerPoolResolver{r} }

// SpruceConfig returns SpruceConfigResolver implementation.
func (r *Resolver) SpruceConfig() SpruceConfigResolver { return &spruceConfigResolver{r} }

type adminSettingsResolver struct{ *Resolver }
type containerPoolResolver struct{ *Resolver }
type spruceConfigResolver struct{ *Resolver }
