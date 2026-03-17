package service

import (
	"errors"
	"skipjd/internal/domain/auth"
	"strings"
	"testing"

	"skipjd/internal/model"

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

func (s *authTestStore) ListUsers() ([]model.User, error) {
	users := make([]model.User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, *user)
	}
	return users, nil
}

func (s *authTestStore) GetUserByID(id uint) (*model.User, error) {
	for _, user := range s.users {
		if user.ID == id {
			copyUser := *user
			return &copyUser, nil
		}
	}
	return nil, nil
}

func (s *authTestStore) GetUserByEmail(email string) (*model.User, error) {
	user, ok := s.users[email]
	if !ok {
		return nil, nil
	}

	copyUser := *user
	return &copyUser, nil
}

func (s *authTestStore) CreateUser(user *model.User) error {
	user.ID = s.nextID
	s.nextID++

	copyUser := *user
	s.users[user.Email] = &copyUser
	return nil
}

func (s *authTestStore) UpdateUser(user *model.User) error {
	copyUser := *user
	s.users[user.Email] = &copyUser
	return nil
}

func (s *authTestStore) DeleteUser(id uint) error {
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

	result, err := service.SignUp(" USER@example.com ", "password123", "  Oak  ")
	if err != nil {
		t.Fatalf("SignUp returned error: %v", err)
	}

	if result.Token == "" {
		t.Fatal("expected token to be returned")
	}
	if parts := strings.Split(result.Token, "."); len(parts) != 3 {
		t.Fatalf("expected jwt with 3 parts, got %d", len(parts))
	}
	if result.User.Email != "user@example.com" {
		t.Fatalf("expected normalized email, got %q", result.User.Email)
	}
	if result.User.Name != "Oak" {
		t.Fatalf("expected trimmed name, got %q", result.User.Name)
	}
	if result.User.Password == "password123" {
		t.Fatal("expected stored password to be hashed")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(result.User.Password), []byte("password123")); err != nil {
		t.Fatalf("expected bcrypt hash to match password: %v", err)
	}
}

func TestAuthServiceSignUpRejectsDuplicateEmail(t *testing.T) {
	store := newAuthTestStore()
	service := NewAuthService(store, "test-secret", 24)

	if _, err := service.SignUp("user@example.com", "password123", "Oak"); err != nil {
		t.Fatalf("first SignUp returned error: %v", err)
	}

	if _, err := service.SignUp("user@example.com", "password123", "Oak"); !errors.Is(err, auth.ErrEmailAlreadyExists) {
		t.Fatalf("expected duplicate email error, got %v", err)
	}
}

func TestAuthServiceSignInReturnsInvalidCredentialsForWrongPassword(t *testing.T) {
	store := newAuthTestStore()
	service := NewAuthService(store, "test-secret", 24)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword returned error: %v", err)
	}

	store.users["user@example.com"] = &model.User{
		ID:       1,
		Email:    "user@example.com",
		Password: string(hashedPassword),
		Name:     "Oak",
		IsActive: true,
	}

	if _, err := service.SignIn("user@example.com", "wrong-password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials error, got %v", err)
	}
}

func TestAuthServiceSignInReturnsInvalidCredentialsForInactiveUser(t *testing.T) {
	store := newAuthTestStore()
	service := NewAuthService(store, "test-secret", 24)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword returned error: %v", err)
	}

	store.users["user@example.com"] = &model.User{
		ID:       1,
		Email:    "user@example.com",
		Password: string(hashedPassword),
		Name:     "Oak",
		IsActive: false,
	}

	if _, err := service.SignIn("user@example.com", "password123"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials error for inactive user, got %v", err)
	}
}
