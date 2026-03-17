package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Issued  int64  `json:"iat"`
	Expires int64  `json:"exp"`
}

func (s *AuthService) generateToken(userID uint, email string) (string, error) {
	issuedAt := time.Now().UTC()
	claims := jwtClaims{
		Subject: strconv.FormatUint(uint64(userID), 10),
		Email:   email,
		Issued:  issuedAt.Unix(),
		Expires: issuedAt.Add(s.jwtExpire).Unix(),
	}

	headerPart, err := encodeJWTPart(jwtHeader{
		Alg: "HS256",
		Typ: "JWT",
	})
	if err != nil {
		return "", err
	}

	claimsPart, err := encodeJWTPart(claims)
	if err != nil {
		return "", err
	}

	unsignedToken := headerPart + "." + claimsPart
	signature := signJWT(unsignedToken, s.jwtSecret)

	return unsignedToken + "." + signature, nil
}

func encodeJWTPart(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal jwt part: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func signJWT(unsignedToken string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(unsignedToken))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
