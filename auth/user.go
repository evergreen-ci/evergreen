package auth

// User describes an Evergreen user and is returned by a UserManager.
type User interface {
	DisplayName() string
	Email() string
	Username() string
}

// UserManager sets and gets user tokens for am implemented authentication mechanism.
type UserManager interface {
	GetUserByToken(token string) (User, error)
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
