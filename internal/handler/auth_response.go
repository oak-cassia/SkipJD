package handler

import "skipjd/internal/service"

type authUserResponse struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type authResponse struct {
	Token string           `json:"token"`
	User  authUserResponse `json:"user"`
}

func toAuthResponse(result *service.AuthResult) authResponse {
	return authResponse{
		Token: result.Token,
		User: authUserResponse{
			ID:    result.User.ID,
			Email: result.User.Email,
			Name:  result.User.Name,
		},
	}
}
