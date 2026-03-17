package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"skipjd/internal/errs"
	"skipjd/internal/model"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userStore UserStore
	jwtSecret []byte
	jwtExpire time.Duration
}

type AuthResult struct {
	Token string
	User  model.User
}

func NewAuthService(userStore UserStore, jwtSecret string, jwtExpireHours int) *AuthService {
	return &AuthService{
		userStore: userStore,
		jwtSecret: []byte(jwtSecret),
		jwtExpire: time.Duration(jwtExpireHours) * time.Hour,
	}
}

func (s *AuthService) SignUp(ctx context.Context, email, password, name string) (*AuthResult, error) {
	email = normalizeEmail(email)
	name = strings.TrimSpace(name)

	existingUser, err := s.userStore.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existingUser != nil {
		return nil, errs.EmailAlreadyExists
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
	if err := s.userStore.CreateUser(ctx, user); err != nil {
		if isDuplicateEmailError(err) {
			return nil, errs.EmailAlreadyExists
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

func (s *AuthService) SignIn(ctx context.Context, email, password string) (*AuthResult, error) {
	email = normalizeEmail(email)
	user, err := s.userStore.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errs.InvalidCredentials
	}
	if !user.IsActive {
		return nil, errs.InvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, errs.InvalidCredentials
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
