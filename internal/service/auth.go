package service

import (
	"errors"
	"strings"

	"skipjd/internal/model"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userStore     UserStore
	tokenProvider TokenProvider
}

var (
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveUser       = errors.New("inactive user")
)

type AuthResult struct {
	Token string
	User  model.User
}

func NewAuthService(userStore UserStore, tokenProvider TokenProvider) *AuthService {
	return &AuthService{
		userStore:     userStore,
		tokenProvider: tokenProvider,
	}
}

func (s *AuthService) SignUp(email, password, name string) (*AuthResult, error) {
	email = normalizeEmail(email)
	name = strings.TrimSpace(name)

	existingUser, err := s.userStore.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}
	if existingUser != nil {
		return nil, ErrEmailAlreadyExists
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Email:    email,
		Password: string(hashedPassword),
		Name:     name,
		IsActive: true,
	}

	if err := s.userStore.CreateUser(user); err != nil {
		if isDuplicateEmailError(err) {
			return nil, ErrEmailAlreadyExists
		}
		return nil, err
	}

	token, err := s.generateToken(user.ID, user.Email)
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		Token: token,
		User:  *user,
	}, nil
}

func (s *AuthService) SignIn(email, password string) (*AuthResult, error) {
	email = normalizeEmail(email)

	user, err := s.userStore.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}
	if !user.IsActive {
		return nil, ErrInactiveUser
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := s.generateToken(user.ID, user.Email)
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		Token: token,
		User:  *user,
	}, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isDuplicateEmailError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func (s *AuthService) generateToken(userID uint, email string) (string, error) {
	if s.tokenProvider == nil {
		return "", errors.New("token provider is not configured")
	}
	return s.tokenProvider.Generate(userID, email)
}
