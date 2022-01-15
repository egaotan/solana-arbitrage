package solscan_tools

import (
	"encoding/json"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

type Transaction struct {
	BlockTime uint64 `json:"blockTime"`
	Slot      uint64 `json:"slot"`
	TxHash    string `json:"txHash"`
}

type TransactionRsp struct {
	Success bool           `json:"succcess"`
	Data    []*Transaction `json:"data"`
}

func GetTransactionsLimit(address solana.PublicKey, before string) []*Transaction {
	req, err := http.NewRequest("GET", SolScanUrl, nil)
	if err != nil {
		panic(err)
	}
	q := url.Values{}
	q.Add("address", address.String())
	if before != "" {
		q.Add("before", before)
	}
	req.Header.Set("Accepts", "application/json")
	req.URL.RawQuery = q.Encode()

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
	var body TransactionRsp
	err = json.Unmarshal(respBody, &body)
	if err != nil {
		panic(err)
	}
	return body.Data
}

func GetTransactionsWithTime(address solana.PublicKey, t uint64) []*Transaction {
	before := ""
	transactions := make([]*Transaction, 0)
	for true {
		txs := GetTransactionsLimit(address, before)
		transactions = append(transactions, txs...)
		tx := txs[len(txs)-1]
		if tx.BlockTime < t {
			break
		}
		fmt.Printf("transaction size: %d\n", len(transactions))
		fmt.Printf("current time: %s\n", time.Unix(int64(tx.BlockTime), 0).Format("2006-01-02 15:04:05"))
		before = tx.TxHash
		time.Sleep(time.Second)
	}
	return transactions
}

func TestFetchTransactionsByAddress(t *testing.T) {
	address := solana.MustPublicKeyFromBase58("Di66GTLsV64JgCCYGVcY21RZ173BHkjJVgPyezNN7P1K")
	tt := uint64(1640785505)
	txs := GetTransactionsWithTime(address, tt)
	{
		infoData, _ := json.MarshalIndent(txs, "", "    ")
		err := os.WriteFile("transactions_by_address.json", infoData, 0644)
		if err != nil {
			panic(err)
		}
	}
}
