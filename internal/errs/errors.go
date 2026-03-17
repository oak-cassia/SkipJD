package errs

type Code string

func (c Code) Error() string {
	return string(c)
}

const (
	InvalidRequest      Code = "INVALID_REQUEST"
	EmailAlreadyExists  Code = "EMAIL_ALREADY_EXISTS"
	InvalidCredentials  Code = "INVALID_CREDENTIALS"
	InternalServerError Code = "INTERNAL_SERVER_ERROR"
)
