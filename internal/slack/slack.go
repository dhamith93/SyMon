package slack

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/joho/godotenv"
)

const SLACK_TEMPLATE_NEW_MSG = `{
	"channel":"{channel}",
	"attachments": [
		{
			"color": "{color}",
			"blocks": [
				{
					"type": "header",
					"text": {
						"type": "plain_text",
						"text": "{subject}"
					}
				},
				{
					"type": "section",
					"text": {
						"type": "plain_text",
						"text": "{content}"
					}
				}
			]
		}
	]
}`

const SLACK_TEMPLATE_RESOLVED = `{
	"channel":"{channel}",
	"ts":"{ts}",
	"attachments": [
		{
			"color": "{color}",
			"blocks": [
				{
					"type": "header",
					"text": {
						"type": "plain_text",
						"text": "{subject}"
					}
				},
				{
					"type": "section",
					"text": {
						"type": "plain_text",
						"text": "{content}"
					}
				}
			]
		}
	]
}`

const ONGOING_COLOR = "#FF3A4C"
const RESOLVED_COLOR = "#00EA6B"
const ONGOING_URL = "https://slack.com/api/chat.postMessage"
const RESOLVED_URL = "https://slack.com/api/chat.update"

type SlackResponse struct {
	Channel string `json:"channel"`
	Ok      bool   `json:"ok"`
	Ts      string `json:"ts"`
	Warning string `json:"warning"`
	Error   string `json:"error"`
}

func SendSlackMessage(subject string, content string, slackChannel string, resolved bool, ts string) (string, error) {
	err := godotenv.Load()
	if err != nil {
		logger.Log("Error", "Error loading .env file")
		return "", err
	}

	msg := ""
	url := ONGOING_URL
	if resolved {
		msg = buildMessage(SLACK_TEMPLATE_RESOLVED, subject, content, slackChannel, RESOLVED_COLOR, ts)
		url = RESOLVED_URL
	} else {
		msg = buildMessage(SLACK_TEMPLATE_NEW_MSG, subject, content, slackChannel, ONGOING_COLOR, ts)
	}

	payload := strings.NewReader(msg)

	req, err := http.NewRequest("POST", url, payload)
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+os.Getenv("SLACK_TOKEN"))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	slackResp := SlackResponse{}
	err = json.Unmarshal(body, &slackResp)
	if err != nil {
		return "", err
	}
	if !slackResp.Ok {
		return "", errors.New("slack error " + slackResp.Error)
	}

	return slackResp.Ts, nil
}

func buildMessage(template string, subject string, content string, channel string, color string, ts string) string {
	var replacer = strings.NewReplacer("{subject}", subject, "{content}", content, "{channel}", channel, "{color}", color, "{ts}", ts)
	return replacer.Replace(template)
}
