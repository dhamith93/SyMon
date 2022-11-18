package auth

import (
	"crypto/rand"
	b64 "encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/golang-jwt/jwt"
)

func ValidToken(token string) bool {
	t, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("error with method")
		}
		return []byte(GetKey(false)), nil
	})
	if err != nil {
		logger.Log("Auth Error", err.Error())
		return false
	}
	return t.Valid
}

func GenerateJWT() (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)

	claims["authorized"] = true
	claims["client"] = "test-client"
	claims["exp"] = time.Now().Add(time.Minute).Unix()

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
	err := os.Setenv("SYMON_KEY", key)
	logger.Log("ERR", err.Error())

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
