package service

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type authClaimsPayload struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

type AuthClaims struct {
	UserID    uint
	Email     string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

func (s *AuthService) generateToken(userID uint, email string) (string, error) {
	issuedAt := time.Now().UTC()
	claims := authClaimsPayload{
		Email: email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(userID), 10),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(s.jwtExpire)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *AuthService) ParseToken(token string) (*AuthClaims, error) {
	var claims authClaimsPayload
	parsedToken, err := jwt.ParseWithClaims(
		token,
		&claims,
		func(parsedToken *jwt.Token) (any, error) {
			if parsedToken.Method == nil || parsedToken.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, errors.New("unexpected signing method")
			}
			return s.jwtSecret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		return nil, err
	}
	if !parsedToken.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil {
		return nil, errors.New("missing required claims")
	}

	userID, err := strconv.ParseUint(claims.Subject, 10, 64)
	if err != nil {
		return nil, errors.New("invalid sub claim")
	}

	return &AuthClaims{
		UserID:    uint(userID),
		Email:     claims.Email,
		IssuedAt:  claims.IssuedAt.Time.UTC(),
		ExpiresAt: claims.ExpiresAt.Time.UTC(),
	}, nil
}
