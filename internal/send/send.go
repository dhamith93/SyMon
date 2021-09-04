package send

import (
	"bytes"
	"compress/gzip"
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

	var buf bytes.Buffer
	g := gzip.NewWriter(&buf)
	if _, err = g.Write(jsonStr); err != nil {
		logger.Log("Error", err.Error())
		return false, -1, ""
	}
	if err = g.Close(); err != nil {
		logger.Log("Error", err.Error())
		return false, -1, ""
	}

	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Token", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")

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
	req.Header.Set("Accept-Encoding", "gzip")

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
	gunzip, err := gzip.NewReader(resp.Body)
	if err != nil {
		logger.Log("Error", err.Error())
		return false, -1, ""
	}
	defer gunzip.Close()
	body, _ := ioutil.ReadAll(gunzip)
	str := string(body)

	if resp.StatusCode == 200 {
		return true, resp.StatusCode, str
	}

	logger.Log("Error", url+" - "+resp.Status+" - "+str)
	return false, resp.StatusCode, str
}
