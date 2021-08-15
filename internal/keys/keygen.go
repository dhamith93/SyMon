package keys

import (
	"bufio"
	"crypto/rand"
	b64 "encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

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
