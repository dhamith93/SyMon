package fileops

import (
	"io/ioutil"

	"github.com/dhamith93/SyMon/internal/logger"
)

// ReadFile read from given file
func ReadFile(path string) string {
	s, err := ioutil.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(s)
}

// WriteFile write to given file
func WriteFile(path string, input string) {
	s := []byte(input)
	err := ioutil.WriteFile(path, s, 0644)
	if err != nil {
		logger.Log("Error", err.Error())
	}
}
