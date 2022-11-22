package pagerduty

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

type Body struct {
	Type    string `json:"type"`
	Details string `json:"details"`
}

type Service struct {
	Id   string `json:"id"`
	Type string `json:"type"`
}

type PagerDutyError struct {
	Message string   `json:"message"`
	Code    int      `json:"code"`
	Errors  []string `json:"errors"`
}

type PagerDutyIncident struct {
	Id      string  `json:"id"`
	Type    string  `json:"type"`
	Title   string  `json:"title"`
	Urgency string  `json:"urgency"`
	Status  string  `json:"status"`
	Body    Body    `json:"body"`
	Service Service `json:"service"`
}

type Incident struct {
	Incident PagerDutyIncident `json:"incident"`
}

type Error struct {
	Error PagerDutyError `json:"error"`
}

const URL = "https://api.pagerduty.com/incidents"

func CreateIncident(incident Incident) (string, error) {
	incident.Incident.Service.Id = os.Getenv("PAGER_DUTY_SERVICE_ID")

	req, err := createRequest(incident, URL, "POST")
	if err != nil {
		return "", err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	pgError := Error{}
	err = json.Unmarshal(body, &pgError)
	if err != nil {
		return "", err
	}
	if pgError.Error.Message != "" {
		return "", errors.New("pagerduty error " + pgError.Error.Message)
	}

	err = json.Unmarshal(body, &incident)
	if err != nil {
		return "", err
	}

	return incident.Incident.Id, nil
}

func UpdateIncident(id string) error {
	payload := strings.NewReader("{\n  \"incident\": {\n    \"type\": \"incident_reference\",\n    \"status\": \"resolved\"\n  }\n}")

	req, err := http.NewRequest("PUT", URL+"/"+id, payload)
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/vnd.pagerduty+json;version=2")
	req.Header.Add("From", os.Getenv("PAGER_DUTY_FROM"))
	req.Header.Add("Authorization", "Token token="+os.Getenv("PAGER_DUTY_API_KEY"))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	pgError := Error{}
	err = json.Unmarshal(body, &pgError)
	if err != nil {
		return err
	}
	if pgError.Error.Message != "" {
		return errors.New("pagerduty error " + pgError.Error.Message)
	}

	return nil
}

func createRequest(incident Incident, url string, method string) (*http.Request, error) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(incident)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, URL, &buf)

	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/vnd.pagerduty+json;version=2")
	req.Header.Add("From", os.Getenv("PAGER_DUTY_FROM"))
	req.Header.Add("Authorization", "Token token="+os.Getenv("PAGER_DUTY_API_KEY"))
	return req, nil
}
