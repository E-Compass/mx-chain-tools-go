package main

import (
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

type tokenBalanceGetter struct {
	proxyURL string
}

func newTokenBalanceGetter(proxyURL string) *tokenBalanceGetter {
	return &tokenBalanceGetter{
		proxyURL: proxyURL,
	}
}

func (tbg *tokenBalanceGetter) getBalance(address, token string) (string, error) {
	tokenID, nonce := getTokenIDAndNonce(token)
	return tbg.fetchTokenBalanceWithRetrial(address, tokenID, nonce)
}

func getTokenIDAndNonce(token string) (string, uint64) {
	tokens := strings.Split(token, "-")
	tokenID := tokens[0] + "-" + tokens[1]

	nonce := big.NewInt(0)
	nonce.SetString(tokens[2], 16)

	return tokenID, nonce.Uint64()
}

func (tbg *tokenBalanceGetter) fetchTokenBalanceWithRetrial(address, tokenID string, nonce uint64) (string, error) {
	ctRetrials := 0

	for ctRetrials < maxRequestsRetrial {
		url := fmt.Sprintf("%s/address/%s/nft/%s/nonce/%d", tbg.proxyURL, address, tokenID, nonce)
		resp, errHttp := http.Get(url)
		body, errBody := tbg.getBody(resp)
		if errHttp == nil && errBody == nil {
			return gjson.Get(body, "data.tokenData.balance").String(), nil
		}

		log.Warn("could not get balance; retrying...",
			"address", address,
			"tokenID", tokenID,
			"token nonce", nonce,
			"error http", errHttp,
			"error body", errBody,
			"num retrials", ctRetrials)

		ctRetrials++
	}

	return "", fmt.Errorf("could not get adress's tokens = %s after num of retrials = %d", address, maxRequestsRetrial)
}

func (tbg *tokenBalanceGetter) getBody(response *http.Response) (string, error) {
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("could not ready bytes from body; error: %w", err)
	}

	bodyStr := string(bodyBytes)
	bodyErr := gjson.Get(bodyStr, "error").String()
	if len(bodyErr) != 0 {
		return "", fmt.Errorf("got error in body response when getting esdt tokens, proxy url: %s, body error: %s", tbg.proxyURL, bodyErr)
	}

	return string(bodyBytes), nil
}
