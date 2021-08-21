package send

import (
	"bytes"
	"io/ioutil"
	"net/http"

	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/logger"
)

func SendPost(url string, json string) (bool, int, string) {
	var jsonStr = []byte(json)
	token, err := auth.GenerateJWT()
	if err != nil {
		logger.Log("Error", err.Error())
		return false, -1, ""
	}

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Token", token)
	req.Header.Set("Content-Type", "application/json")

	return send(req, url)
}

func SendGet(url string) (bool, int, string) {
	token, err := auth.GenerateJWT()
	if err != nil {
		logger.Log("Error", err.Error())
		return false, -1, ""
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Token", token)

	return send(req, url)
}

func send(req *http.Request, url string) (bool, int, string) {
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Log("Error", err.Error())
		return false, -1, ""
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	str := string(body)

	if resp.StatusCode == 200 {
		return true, resp.StatusCode, str
	}

	logger.Log("Error", url+" - "+resp.Status+" - "+str)
	return false, resp.StatusCode, str
}
