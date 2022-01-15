package tools

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

type TokenList struct {
	Name      string               `json:"name"`
	Tags      map[string]*TokenTag `json:"tags"`
	TimeStamp string               `json:"timestamp"`
	Tokens    []*Token             `json:"tokens"`
}

type TokenTag struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Extensions struct {
	Website   string `json:"website"`
	Telegram  string `json:"telegram"`
	Twitter   string `json:"twitter"`
	Discord   string `json:"discord"`
	Instagram string `json:"instagram"`
}

type Token struct {
	ChainId    int         `json:"chainId"`
	Address    string      `json:"address"`
	Symbol     string      `json:"symbol"`
	Name       string      `json:"name"`
	Decimals   int         `json:"decimals"`
	LogoURI    string      `jons:"logoURI"`
	Tags       []string    `json:"tags"`
	Extensions *Extensions `json:"extensions"`
}

func (tl *TokenList) GetToken(address string) *Token {
	for _, token := range tl.Tokens {
		if token.Address == address {
			return token
		}
	}
	return &Token{
		ChainId:    0,
		Address:    "",
		Symbol:     "",
		Name:       "",
		Decimals:   0,
		LogoURI:    "",
		Tags:       nil,
		Extensions: nil,
	}
}

var (
	gTokenList *TokenList
	FilePath   string
)

func readFile(fileName string) ([]byte, error) {
	file, err := os.OpenFile(fileName, os.O_RDONLY, 0666)
	if err != nil {
		return nil, fmt.Errorf("OpenFile %s error %s", fileName, err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			fmt.Printf("FilePath %s close error %s", fileName, err)
		}
	}()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll %s error %s", fileName, err)
	}
	return data, nil
}

func InitTokenList() (*TokenList, error) {
	data, err := readFile(FilePath)
	if err != nil {
		return nil, fmt.Errorf("read file err: %v\n", err)
	}
	var tokenList TokenList
	err = json.Unmarshal(data, &tokenList)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal TestConfig:%s error:%s", data, err)
	}
	return &tokenList, nil
}

func Instance() *TokenList {
	if gTokenList == nil {
		gTokenList, _ = InitTokenList()
	}
	return gTokenList
}
