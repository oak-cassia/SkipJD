package service

type TokenProvider interface {
	Generate(userID uint, email string) (string, error)
}
