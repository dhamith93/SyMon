package client

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"symon/util"
)

type Remote struct {
	Name string
	Url  string
	Port string
	Key  string
}

func Get(who string, what string) string {
	remote := getRemote(who)

	if remote.Key == "" || remote.Url == "" || remote.Port == "" {
		panic("Cannot parse remote.json")
	}

	client := &http.Client{}
	req, _ := http.NewRequest("GET", strings.Trim(remote.Url, "/")+":"+remote.Port+"/"+what, nil)
	req.Header.Set("Key", remote.Key)
	res, err := client.Do(req)

	if err != nil {
		util.Log("Error", err.Error())
		panic(err)
	}

	if res.StatusCode != 200 {
		util.Log("Error", "Response status is: "+res.Status)
		panic("Response status is: " + res.Status)
	}

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		util.Log("Error", err.Error())
		panic(err)
	}

	return string(body)
}

func getRemote(name string) Remote {
	file, err := ioutil.ReadFile("remote.json")
	remotes := []Remote{}

	if err != nil {
		util.Log("Error", "Cannot read remote.json")
		panic("Cannot read remote.json")
	}

	err = json.Unmarshal([]byte(file), &remotes)

	if err != nil {
		util.Log("Error", "Cannot parse remote.json")
		panic("Cannot parse remote.json")
	}

	for _, remote := range remotes {
		if remote.Name == name {
			return remote
		}
	}

	return Remote{}
}
