package service

import (
	"context"
	"skipjd/internal/errs"
	"strings"
	"testing"

	"skipjd/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type authTestStore struct {
	users  map[string]*model.User
	nextID uint
}

func newAuthTestStore() *authTestStore {
	return &authTestStore{
		users:  make(map[string]*model.User),
		nextID: 1,
	}
}

func (s *authTestStore) ListUsers(ctx context.Context) ([]model.User, error) {
	users := make([]model.User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, *user)
	}
	return users, nil
}

func (s *authTestStore) GetUserByID(ctx context.Context, id uint) (*model.User, error) {
	for _, user := range s.users {
		if user.ID == id {
			copyUser := *user
			return &copyUser, nil
		}
	}
	return nil, nil
}

func (s *authTestStore) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	user, ok := s.users[email]
	if !ok {
		return nil, nil
	}

	copyUser := *user
	return &copyUser, nil
}

func (s *authTestStore) CreateUser(ctx context.Context, user *model.User) error {
	user.ID = s.nextID
	s.nextID++

	copyUser := *user
	s.users[user.Email] = &copyUser
	return nil
}

func (s *authTestStore) UpdateUser(ctx context.Context, user *model.User) error {
	copyUser := *user
	s.users[user.Email] = &copyUser
	return nil
}

func (s *authTestStore) DeleteUser(ctx context.Context, id uint) error {
	for email, user := range s.users {
		if user.ID == id {
			delete(s.users, email)
			return nil
		}
	}
	return nil
}

func TestAuthServiceSignUpHashesPasswordAndReturnsToken(t *testing.T) {
	store := newAuthTestStore()
	service := NewAuthService(store, "test-secret", 24)

	result, err := service.SignUp(context.Background(), " USER@example.com ", "password123", "  Oak  ")
	require.NoError(t, err)
	require.NotEmpty(t, result.Token)
	assert.Len(t, strings.Split(result.Token, "."), 3)
	assert.Equal(t, "user@example.com", result.User.Email)
	assert.Equal(t, "Oak", result.User.Name)
	assert.NotEqual(t, "password123", result.User.Password)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(result.User.Password), []byte("password123")))
}

func TestAuthServiceSignUpRejectsDuplicateEmail(t *testing.T) {
	store := newAuthTestStore()
	service := NewAuthService(store, "test-secret", 24)

	_, err := service.SignUp(context.Background(), "user@example.com", "password123", "Oak")
	require.NoError(t, err)

	_, err = service.SignUp(context.Background(), "user@example.com", "password123", "Oak")
	require.ErrorIs(t, err, errs.EmailAlreadyExists)
}

func TestAuthServiceSignInReturnsInvalidCredentialsForWrongPassword(t *testing.T) {
	store := newAuthTestStore()
	service := NewAuthService(store, "test-secret", 24)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	require.NoError(t, err)

	store.users["user@example.com"] = &model.User{
		ID:       1,
		Email:    "user@example.com",
		Password: string(hashedPassword),
		Name:     "Oak",
		IsActive: true,
	}

	_, err = service.SignIn(context.Background(), "user@example.com", "wrong-password")
	require.ErrorIs(t, err, errs.InvalidCredentials)
}

func TestAuthServiceSignInReturnsInvalidCredentialsForInactiveUser(t *testing.T) {
	store := newAuthTestStore()
	service := NewAuthService(store, "test-secret", 24)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	require.NoError(t, err)

	store.users["user@example.com"] = &model.User{
		ID:       1,
		Email:    "user@example.com",
		Password: string(hashedPassword),
		Name:     "Oak",
		IsActive: false,
	}

	_, err = service.SignIn(context.Background(), "user@example.com", "password123")
	require.ErrorIs(t, err, errs.InvalidCredentials)
}
