package evergreen

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLDAPRoleMapAddAndRemove(t *testing.T) {
	env, err := NewEnvironment(context.Background(), filepath.Join("testdata", "smoke_config.yml"), nil)
	require.NoError(t, err)
	SetEnvironment(env)
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)
	s := &Settings{}
	m := &LDAPRoleMap{}

	expectedMappings := map[string]string{"group1": "role1"}
	err = m.Add("group1", "role1")
	require.NoError(t, err)
	require.NoError(t, coll.FindOne(ctx, byId(s.SectionId())).Decode(s))
	for _, mapping := range s.LDAPRoleMap {
		require.Equal(t, expectedMappings[mapping.LDAPGroup], mapping.RoleID)
	}

	expectedMappings["group2"] = "role2"
	err = m.Add("group2", "role2")
	require.NoError(t, err)
	require.NoError(t, coll.FindOne(ctx, byId(s.SectionId())).Decode(s))
	for _, mapping := range s.LDAPRoleMap {
		require.Equal(t, expectedMappings[mapping.LDAPGroup], mapping.RoleID)
	}

	expectedMappings["group1"] = "role2"
	err = m.Add("group1", "role2")
	require.NoError(t, err)
	require.NoError(t, coll.FindOne(ctx, byId(s.SectionId())).Decode(s))
	for _, mapping := range s.LDAPRoleMap {
		require.Equal(t, expectedMappings[mapping.LDAPGroup], mapping.RoleID)
	}

	delete(expectedMappings, "group1")
	err = m.Remove("group1")
	require.NoError(t, err)
	require.NoError(t, coll.FindOne(ctx, byId(s.SectionId())).Decode(s))
	for _, mapping := range s.LDAPRoleMap {
		require.Equal(t, expectedMappings[mapping.LDAPGroup], mapping.RoleID)
	}

	err = m.Remove("group1")
	require.NoError(t, err)
}
