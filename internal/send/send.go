package send

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/logger"
)

func SendPost(url string, json string) bool {
	var jsonStr = []byte(json)
	token, err := auth.GenerateJWT()
	if err != nil {
		logger.Log("Error", err.Error())
		return false
	}

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Token", token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Log("Error", err.Error())
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true
	}

	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println(string(body))
	logger.Log("Error", url+" - "+resp.Status+" - "+string(body))
	return false
}
