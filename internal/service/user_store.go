package service

import "skipjd/internal/model"

type UserStore interface {
	ListUsers() ([]model.User, error)
	GetUserByID(id uint) (*model.User, error)
	GetUserByEmail(email string) (*model.User, error)
	CreateUser(user *model.User) error
	UpdateUser(user *model.User) error
	DeleteUser(id uint) error
}
