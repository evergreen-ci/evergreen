// Code generated by rest/model/codegen.go. DO NOT EDIT.

package model

import "github.com/evergreen-ci/evergreen/model/user"

type APIDBUser struct {
	UserID      *string  `json:"user_id"`
	DisplayName *string  `json:"display_name"`
	OnlyApi     bool     `json:"only_api"`
	Roles       []string `json:"roles"`
}

func APIDBUserBuildFromService(t user.DBUser) *APIDBUser {
	m := APIDBUser{}
	m.DisplayName = StringStringPtr(t.DispName)
	m.Roles = ArrstringArrstring(t.SystemRoles)
	m.UserID = StringStringPtr(t.Id)
	return &m
}

func APIDBUserToService(m APIDBUser) *user.DBUser {
	out := &user.DBUser{}
	out.DispName = StringPtrString(m.DisplayName)
	out.Id = StringPtrString(m.UserID)
	out.SystemRoles = ArrstringArrstring(m.Roles)
	return out
}
