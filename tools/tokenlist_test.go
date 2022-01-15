package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

type TokenInfo struct {
	Symbol  string
	Name    string
	Decimal uint64
	Price   decimal.Decimal
}

func TestFetchTokenList(t *testing.T) {
	req, err := http.NewRequest("GET", "https://cdn.jsdelivr.net/gh/solana-labs/token-list@main/src/tokens/solana.tokenlist.json", nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Accepts", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		panic(fmt.Errorf("response status code: %d", resp.StatusCode))
	}
	respBody, _ := ioutil.ReadAll(resp.Body)
	var tokenList TokenList
	err = json.Unmarshal(respBody, &tokenList)
	if err != nil {
		panic(err)
	}
	infoJson, _ := json.MarshalIndent(tokenList, "", "    ")
	name := fmt.Sprintf("./tools/solana.tokenlist.json")
	err = os.WriteFile(name, infoJson, 0644)
	if err != nil {
		panic(err)
	}
}

func TestInitToken(t *testing.T) {
	FilePath = "./tools/solana.tokenlist.json"
	tokenList, err := InitTokenList()
	if err != nil {
		panic(err)
	}
	infoJson, err := os.ReadFile(config.MarketUsableTokensFile)
	if err != nil {
		panic(err)
	}
	defiTokens := make(map[solana.PublicKey]bool)
	err = json.Unmarshal(infoJson, &defiTokens)
	if err != nil {
		panic(err)
	}
	tokens := make(map[string]*TokenInfo)
	for token, _ := range defiTokens {
		item := tokenList.GetToken(token.String())
		if item.Address == "" {
			item.Name = token.String()
			item.Symbol = token.String()
		}
		tokens[token.String()] = &TokenInfo{
			Symbol:  item.Symbol,
			Name:    item.Name,
			Decimal: xxx(item.Decimals),
		}
	}
	{
		infoData, _ := json.MarshalIndent(tokens, "", "    ")
		err = os.WriteFile(config.TokensFile, infoData, 0644)
		if err != nil {
			panic(err)
		}
	}
}

type Holder struct {
	Address  string `json:"address"`
	Amount   uint64 `json:"amount"`
	Decimals int    `json:"decimals"`
	Owner    string `json:"owner"`
	Rank     int    `json:"rank"`
}

type Data struct {
	Total  int      `json:"total"`
	Result []Holder `json:"result"`
}

type Response struct {
	Success bool `json:"succcess"`
	Data    Data `json:"data"`
}

func TestInitUser(t *testing.T) {
	swapAccountData, err := os.ReadFile(config.MarketUsablePoolAccounts)
	if err != nil {
		panic(err)
	}
	swapAccounts := make(map[solana.PublicKey]bool)
	err = json.Unmarshal(swapAccountData, &swapAccounts)
	if err != nil {
		panic(err)
	}
	tokenData, err := os.ReadFile(config.MarketUsableTokensFile)
	if err != nil {
		panic(err)
	}
	tokens := make(map[solana.PublicKey]bool)
	err = json.Unmarshal(tokenData, &tokens)
	if err != nil {
		panic(err)
	}

	rpcClient := rpc.New("https://solana-api.projectserum.com")
	client := &http.Client{}
	users := make(map[solana.PublicKey]solana.PublicKey)
	for token, _ := range tokens {
		req, err := http.NewRequest("GET", "https://api.solscan.io/token/holders", nil)
		if err != nil {
			panic(err)
		}
		q := url.Values{}
		q.Add("token", token.String())
		q.Add("offset", "0")
		q.Add("size", "20")

		req.Header.Set("Accepts", "application/json")
		req.URL.RawQuery = q.Encode()

		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			panic(fmt.Errorf("response status code: %d", resp.StatusCode))
		}
		respBody, _ := ioutil.ReadAll(resp.Body)
		respBody1 := strings.Replace(string(respBody), "\"amount\":\"", "\"amount\":", 100)
		respBody1 = strings.Replace(string(respBody1), "\",\"decimals\"", ",\"decimals\"", 100)
		var body Response
		err = json.Unmarshal([]byte(respBody1), &body)
		if err != nil {
			panic(err)
		}
		for _, item := range body.Data.Result {
			user := solana.MustPublicKeyFromBase58(item.Address)
			userInfo, err := rpcClient.GetAccountInfo(context.Background(), user)
			if err != nil {
				panic(err)
			}
			spl_token_info, err := decodeAccount(userInfo.Value)
			if err != nil {
				panic(err)
			}
			ownerBalance, err := rpcClient.GetBalance(context.Background(), spl_token_info.Owner, rpc.CommitmentFinalized)
			if err != nil {
				panic(err)
			}
			if _, ok := swapAccounts[solana.MustPublicKeyFromBase58(item.Address)]; ok {
				continue
			}
			if ownerBalance.Value < 100000000 {
				continue
			}
			fmt.Printf("token: %s, user: %s, owner: %s\n", token.String(), user.String(), spl_token_info.Owner.String())
			users[token] = user
			break
		}
	}
	{
		infoData, _ := json.MarshalIndent(users, "", "    ")
		err = os.WriteFile(config.UsersSimulateFile, infoData, 0644)
		if err != nil {
			panic(err)
		}
	}
}

func TestInitOpenOrder(t *testing.T) {
	markets := make(map[solana.PublicKey]map[solana.PublicKey]bool)
	marketsJson, err := os.ReadFile(config.MarketsFile)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(marketsJson, &markets)
	if err != nil {
		panic(err)
	}
	operorders := make(map[solana.PublicKey]solana.PublicKey)
	for _, v1 := range markets {
		for v2, _ := range v1 {
			wallet := solana.NewWallet()
			operorders[v2] = wallet.PublicKey()
		}
	}
	{
		infoData, _ := json.MarshalIndent(operorders, "", "    ")
		err = os.WriteFile(config.MarketOpenOrdersSimulateFile, infoData, 0644)
		if err != nil {
			panic(err)
		}
	}
}
func xxx(a int) uint64 {
	item := uint64(1)
	for i := 0; i < a; i++ {
		item *= 10
	}
	return item
}

type SerumMarket struct {
	Address   string `json:"address"`
	ProgramId string `json:"programId"`
}

func TestSerumMarkets(t *testing.T) {
	serumMarketsData, err := os.ReadFile("./serum_markets.json")
	if err != nil {
		panic(err)
	}
	serumMarkets := make([]*SerumMarket, 0)
	err = json.Unmarshal(serumMarketsData, &serumMarkets)
	if err != nil {
		panic(err)
	}
	xx := make(map[solana.PublicKey]map[solana.PublicKey]bool)
	for _, serumMarket := range serumMarkets {
		programId := solana.MustPublicKeyFromBase58(serumMarket.ProgramId)
		address := solana.MustPublicKeyFromBase58(serumMarket.Address)
		if _, ok := xx[programId]; !ok {
			xx[programId] = make(map[solana.PublicKey]bool)
		}
		xx[programId][address] = true
	}
	{
		infoData, _ := json.MarshalIndent(xx, "", "    ")
		fmt.Printf("%s", string(infoData))
	}
}

type Orcavolume struct {
	Day decimal.Decimal `json:"day,string"`
}

type OrcaPair struct {
	PoolId      string      `json:"poolId"`
	PoolAccount string      `json:"poolAccount"`
	Volume      *Orcavolume `json:"volume"`
}

func TestOrcaMarkets(t *testing.T) {
	orcaMarketsData, err := os.ReadFile("./orca_markets.json")
	if err != nil {
		panic(err)
	}
	orcaMarkets := make(map[string]*OrcaPair)
	err = json.Unmarshal(orcaMarketsData, &orcaMarkets)
	if err != nil {
		panic(err)
	}
	selectOrcaMarket := make(map[string]bool)
	for k, market := range orcaMarkets {
		if market.Volume.Day.Cmp(decimal.NewFromInt(1000000)) > 0 {
			selectOrcaMarket[market.PoolAccount] = true
		} else {
			delete(orcaMarkets, k)
		}
	}
	{
		infoData, _ := json.MarshalIndent(orcaMarkets, "", "    ")
		err = os.WriteFile("./orca_used_markets1.json", infoData, 0644)
		if err != nil {
			panic(err)
		}
	}
	{
		infoData, _ := json.MarshalIndent(selectOrcaMarket, "", "    ")
		err = os.WriteFile("./orca_used_markets.json", infoData, 0644)
		if err != nil {
			panic(err)
		}
	}
}
