package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/ElrondNetwork/elrond-sdk-erdgo/blockchain"
	"github.com/ElrondNetwork/elrond-sdk-erdgo/builders"
	"github.com/ElrondNetwork/elrond-sdk-erdgo/core"
	"github.com/ElrondNetwork/elrond-sdk-erdgo/data"
	"github.com/ElrondNetwork/elrond-sdk-erdgo/interactors"
	"github.com/ElrondNetwork/elrond-tools-go/tokensRemover/metaDataRemover/config"
)

func createShardTxs(
	outFile string,
	cfg *config.Config,
	shardPemsDataMap map[uint32]*pkAddress,
	shardTxsDataMap map[uint32][][]byte,
) error {
	if len(shardPemsDataMap) != len(shardTxsDataMap) {
		return fmt.Errorf("provided invalid input; expected number of pem files = number of shards in tokens input; got num shard tokens = %d, num pem files = %d",
			len(shardPemsDataMap), len(shardTxsDataMap))
	}

	args := blockchain.ArgsElrondProxy{
		ProxyURL:            cfg.ProxyUrl,
		CacheExpirationTime: time.Minute,
		EntityType:          core.Proxy,
	}

	proxy, err := blockchain.NewElrondProxy(args)
	if err != nil {
		return err
	}

	txBuilder, err := builders.NewTxBuilder(blockchain.NewTxSigner())
	if err != nil {
		return err
	}

	ti, err := interactors.NewTransactionInteractor(proxy, txBuilder)
	if err != nil {
		return err
	}

	err = createOutputFileIfDoesNotExist(outFile)
	if err != nil {
		return err
	}

	for shardID, txsData := range shardTxsDataMap {
		pemData, found := shardPemsDataMap[shardID]
		if !found {
			return fmt.Errorf("no pem data provided for shard = %d", shardID)
		}

		log.Info("starting to create txs", "shardID", shardID, "num of txs", len(txsData))
		txsInShard, err := createTxs(pemData, proxy, ti, txsData, cfg.GasLimit)
		if err != nil {
			return err
		}

		file := outFile + "/txsShard" + strconv.Itoa(int(shardID)) + ".json"
		log.Info("saving txs", "shardID", shardID, "file", file)
		err = saveResult(txsInShard, file)
	}

	return nil
}

func createTxs(
	pemData *pkAddress,
	proxy proxyProvider,
	txInteractor transactionInteractor,
	txsData [][]byte,
	gasLimit uint64,
) ([]*data.Transaction, error) {
	transactionArguments, err := getDefaultTxsArgs(proxy, pemData.address, gasLimit)
	if err != nil {
		return nil, err
	}

	txs := make([]*data.Transaction, 0, len(txsData))
	for _, txData := range txsData {
		transactionArguments.Data = txData
		tx, err := txInteractor.ApplySignatureAndGenerateTx(pemData.privateKey, *transactionArguments)
		if err != nil {
			return nil, err
		}

		txs = append(txs, tx)
		transactionArguments.Nonce++
	}

	return txs, nil
}

func getDefaultTxsArgs(proxy proxyProvider, address core.AddressHandler, gasLimit uint64) (*data.ArgCreateTransaction, error) {
	netConfigs, err := proxy.GetNetworkConfig(context.Background())
	if err != nil {
		return nil, err

	}

	transactionArguments, err := proxy.GetDefaultTransactionArguments(context.Background(), address, netConfigs)
	if err != nil {
		return nil, err
	}

	transactionArguments.RcvAddr = address.AddressAsBech32String() // send to self
	transactionArguments.Value = "0"
	transactionArguments.GasLimit = gasLimit

	return &transactionArguments, nil
}

func createOutputFileIfDoesNotExist(outFile string) error {
	_, err := os.Stat(outFile)

	if os.IsNotExist(err) {
		errDir := os.MkdirAll(outFile, os.ModePerm)
		if errDir != nil {
			return err
		}
	}

	return nil
}

func saveResult(txs []*data.Transaction, outfile string) error {
	jsonBytes, err := json.MarshalIndent(txs, "", " ")
	if err != nil {
		return err
	}

	log.Info("writing result in", "file", outfile)
	err = ioutil.WriteFile(outfile, jsonBytes, fs.FileMode(outputFilePerms))
	if err != nil {
		return err
	}
	return nil
}
