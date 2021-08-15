package auth

import (
	"bufio"
	"crypto/rand"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/command"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/golang-jwt/jwt"
)

func CheckAuth(endpoint func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header["Token"] != nil {
			token, err := jwt.Parse(r.Header["Token"][0], func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("error with method")
				}
				return []byte(GetKey()), nil
			})

			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode("Unauthorized")
				logger.Log("Auth Error", err.Error())
			}

			if token.Valid {
				endpoint(w, r)
			}
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode("Unauthorized")
			ip, _ := command.GetIncomingIPAddr(r)
			logger.Log("Auth Error", "Unauthorized request from "+ip)
		}
	})
}

func GenerateJWT() (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)

	claims["authorized"] = true
	claims["client"] = "test-client"
	claims["exp"] = time.Now().Add(time.Minute).Unix()

	tokenString, err := token.SignedString([]byte(GetKey()))

	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// GetKey returns key from file if exist or generate key
func GetKey() string {
	if _, err := os.Stat("key"); err == nil {
		b, err := ioutil.ReadFile("key")
		if err != nil {
			log.Println(err.Error())
			panic(err)
		}
		return strings.TrimSpace(string(b))
	}

	key := keyGen()
	file, err := os.Create("key")
	if err != nil {
		log.Println(err.Error())
		panic(err)
	}
	w := bufio.NewWriter(file)
	_, err = fmt.Fprintf(w, "%v", key)
	if err != nil {
		log.Println(err.Error())
		panic(err)
	}
	w.Flush()

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
