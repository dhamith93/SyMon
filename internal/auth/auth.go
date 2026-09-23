package auth

import (
	"crypto/rand"
	b64 "encoding/base64"
	"os"
	"time"

	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/golang-jwt/jwt/v5"
)

func ValidToken(token string) bool {
	t, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		return []byte(GetKey(false)), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		logger.Log("Auth Error", err.Error())
		return false
	}
	return t.Valid
}

func GenerateJWT() (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"authorized": true,
		"client":     "test-client",
		"exp":        time.Now().Add(time.Minute).Unix(),
	})

	tokenString, err := token.SignedString([]byte(GetKey(false)))

	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func GetKey(generateIfNotFound bool) string {
	key := os.Getenv("SYMON_KEY")

	if len(key) > 0 {
		return key
	}

	if !generateIfNotFound {
		return ""
	}

	key = keyGen()
	if err := os.Setenv("SYMON_KEY", key); err != nil {
		logger.Log("error", err.Error())
	}

	return key
}

func keyGen() string {
	key := make([]byte, 64)
	_, err := rand.Read(key)
	if err != nil {
		panic(err)
	}
	return b64.StdEncoding.EncodeToString(key)
}
