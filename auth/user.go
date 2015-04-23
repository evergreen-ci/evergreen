package auth

import ()

type MCIUser interface {
	DisplayName() string
	Email() string
	Username() string
}

type UserManager interface {
	GetUserByToken(token string) (MCIUser, error)
	CreateUserToken(username, password string) (string, error)
}

type simpleUser struct {
	UserId       string
	Name         string
	EmailAddress string
}

func (u *simpleUser) DisplayName() string {
	return u.Name
}

func (u *simpleUser) Email() string {
	return u.EmailAddress
}

func (u *simpleUser) Username() string {
	return u.UserId
}
