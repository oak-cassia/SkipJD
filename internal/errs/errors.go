package errs

type Code string

func (c Code) Error() string {
	return string(c)
}

const (
	InvalidRequest      Code = "INVALID_REQUEST"
	EmailAlreadyExists  Code = "EMAIL_ALREADY_EXISTS"
	InvalidCredentials  Code = "INVALID_CREDENTIALS"
	InvalidToken        Code = "INVALID_TOKEN"
	InternalServerError Code = "INTERNAL_SERVER_ERROR"
)
