package env

import (
	"encoding/json"
	"github.com/egaotan/solana-arbitrage/config"
	"github.com/gagliardetto/solana-go"
	"os"
)

func (e *Env) loadTokensUser() {
	infoJson, err := os.ReadFile(config.UsersFile)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(infoJson, &e.tokensUser)
	if err != nil {
		panic(err)
	}
}

func (e *Env) loadTokensUserSimulate() {
	infoJson, err := os.ReadFile(config.UsersSimulateFile)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(infoJson, &e.tokensUserSimulate)
	if err != nil {
		panic(err)
	}
}

func (e *Env) loadUsersOwner() {
	infoJson, err := os.ReadFile(config.UsersOwnerFile)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(infoJson, &e.usersOwner)
	if err != nil {
		panic(err)
	}
}

func (e *Env) loadUsersOwnerSimulate() {
	infoJson, err := os.ReadFile(config.UserOwnerSimulateFile)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(infoJson, &e.usersOwnerSimulate)
	if err != nil {
		panic(err)
	}
}

func (e *Env) TokenUser(token solana.PublicKey) solana.PublicKey {
	item, ok := e.tokensUser[token]
	if !ok {
		return solana.PublicKey{}
	}
	return item
}

func (e *Env) TokenUserSimulate(token solana.PublicKey) solana.PublicKey {
	item, ok := e.tokensUserSimulate[token]
	if !ok {
		return solana.PublicKey{}
	}
	return item
}

func (e *Env) UsersOwner(user solana.PublicKey) solana.PublicKey {
	item, ok := e.usersOwner[user]
	if !ok {
		return solana.PublicKey{}
	}
	return item
}

func (e *Env) UsersOwnerSimulate(user solana.PublicKey) solana.PublicKey {
	item, ok := e.usersOwnerSimulate[user]
	if !ok {
		return solana.PublicKey{}
	}
	return item
}
