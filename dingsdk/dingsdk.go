package dingsdk

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type DingContent struct {
	Content string `json:"content"`
}
type DingAt struct {
	IsAtAll bool `json:"isAtAll"`
}
type DingNotify struct {
	MsgType string      `json:"msgtype"`
	Text    DingContent `json:"text"`
	At      DingAt      `json:"at"`
}

type DingResult struct {
	ErrCode int64  `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type DingSdk struct {
	url string
}

func NewDingSdk(url string) *DingSdk {
	sdk := &DingSdk{
		url: url,
	}
	return sdk
}

func (sdk *DingSdk) Notify(notify *DingNotify) (*DingResult, error) {
	requestJson, _ := json.Marshal(notify)
	req, err := http.NewRequest("POST", sdk.url, strings.NewReader(string(requestJson)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accepts", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("response status code: %d", resp.StatusCode)
	}
	respBody, _ := ioutil.ReadAll(resp.Body)
	dingResult := new(DingResult)
	err = json.Unmarshal(respBody, dingResult)
	if err != nil {
		return nil, err
	}
	if dingResult.ErrCode != 0 || dingResult.ErrMsg != "ok" {
		return nil, fmt.Errorf("code: %d, err: %s", dingResult.ErrCode, dingResult.ErrMsg)
	}
	return dingResult, nil
}
