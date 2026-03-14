package service

type UserService struct {
	userStore UserStore
}

func NewUserService(userStore UserStore) *UserService {
	return &UserService{userStore: userStore}
}
