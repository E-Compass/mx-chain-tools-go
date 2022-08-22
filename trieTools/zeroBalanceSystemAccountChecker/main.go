package main

import (
	"encoding/json"
	"fmt"
	"github.com/ElrondNetwork/elrond-go-core/core/pubkeyConverter"
	logger "github.com/ElrondNetwork/elrond-go-logger"
	"github.com/ElrondNetwork/elrond-tools-go/elasticreindexer/config"
	"github.com/ElrondNetwork/elrond-tools-go/elasticreindexer/elastic"
	sysAccConfig "github.com/ElrondNetwork/elrond-tools-go/trieTools/zeroBalanceSystemAccountChecker/config"
	"github.com/pelletier/go-toml"
	"github.com/urfave/cli"

	"github.com/ElrondNetwork/elrond-tools-go/trieTools/trieToolsCommon"
	vmcommon "github.com/ElrondNetwork/elrond-vm-common"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

const (
	logFilePrefix   = "system-account-zero-tokens-balance-checker"
	addressLength   = 32
	outputFilePerms = 0644
	tomlFile        = "./config.toml"
)

type crossTokenChecker interface {
	crossCheckExtraTokens(tokens map[string]struct{}) ([]string, error)
}

func main() {
	app := cli.NewApp()
	app.Name = "Tokens exporter CLI app"
	app.Usage = "This is the entry point for the tool that checks which tokens are not used anymore(only stored in system account)"
	app.Flags = getFlags()
	app.Authors = []cli.Author{
		{
			Name:  "The Elrond Team",
			Email: "contact@elrond.com",
		},
	}

	app.Action = func(c *cli.Context) error {
		return startProcess(c)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
		return
	}
}

func startProcess(c *cli.Context) error {
	flagsConfig := getFlagsConfig(c)

	_, errLogger := trieToolsCommon.AttachFileLogger(log, logFilePrefix, flagsConfig.ContextFlagsConfig)
	if errLogger != nil {
		return errLogger
	}

	log.Info("sanity checks...")

	err := logger.SetLogLevel(flagsConfig.LogLevel)
	if err != nil {
		return err
	}

	log.Info("starting processing trie", "pid", os.Getpid())

	addressTokensMap, err := readInputs(flagsConfig.TokensDirectory)
	if err != nil {
		return err
	}

	extraTokens, err := exportSystemAccZeroTokensBalances(addressTokensMap)
	if err != nil {
		return err
	}

	if flagsConfig.CrossCheck {
		err = crossCheckExtraTokens(extraTokens)
		if err != nil {
			return err
		}
	}

	err = saveResult(extraTokens, flagsConfig.Outfile)
	if err != nil {
		return err
	}

	return nil
}

func readInputs(tokensDir string) (map[string]map[string]struct{}, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(workingDir, tokensDir)
	contents, err := ioutil.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	allAddressesTokensMap := make(map[string]map[string]struct{})
	for _, file := range contents {
		if file.IsDir() {
			continue
		}

		addressTokensMapInCurrFile, err := getFileContent(filepath.Join(fullPath, file.Name()))
		if err != nil {
			return nil, err
		}

		merge(allAddressesTokensMap, addressTokensMapInCurrFile)
		log.Info("read data from",
			"file", file.Name(),
			"num addresses in current file", len(addressTokensMapInCurrFile),
			"num addresses in total, after merge", len(allAddressesTokensMap))
	}

	return allAddressesTokensMap, nil
}

func getFileContent(file string) (map[string]map[string]struct{}, error) {
	jsonFile, err := os.Open(file)
	if err != nil {
		return nil, err
	}

	bytesFromJson, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return nil, err
	}

	addressTokensMapInCurrFile := make(map[string]map[string]struct{})
	err = json.Unmarshal(bytesFromJson, &addressTokensMapInCurrFile)
	if err != nil {
		return nil, err
	}

	return addressTokensMapInCurrFile, nil
}

func merge(dest, src map[string]map[string]struct{}) {
	for addressSrc, tokensSrc := range src {
		_, existsInDest := dest[addressSrc]
		if !existsInDest {
			dest[addressSrc] = tokensSrc
		} else {
			log.Debug("same address found in multiple files", "address", addressSrc)
			addTokensInDestAddress(tokensSrc, dest, addressSrc)
		}
	}
}

func addTokensInDestAddress(tokens map[string]struct{}, dest map[string]map[string]struct{}, address string) {
	for token := range tokens {
		dest[address][token] = struct{}{}
	}
}

func exportSystemAccZeroTokensBalances(allAddressesTokensMap map[string]map[string]struct{}) (map[string]struct{}, error) {
	addressConverter, err := pubkeyConverter.NewBech32PubkeyConverter(addressLength, log)
	if err != nil {
		return nil, err
	}

	systemSCAddress := addressConverter.Encode(vmcommon.SystemAccountAddress)
	allTokensInSystemSCAddress, foundSystemSCAddress := allAddressesTokensMap[systemSCAddress]
	if !foundSystemSCAddress {
		return nil, fmt.Errorf("no system account address(%s) found", systemSCAddress)
	}

	allTokens := getAllTokensWithoutSystemAccount(allAddressesTokensMap, systemSCAddress)
	log.Info("found",
		"total num of tokens in all addresses", len(allTokens),
		"total num of tokens in system sc address", len(allTokensInSystemSCAddress))

	return getExtraTokens(allTokens, allTokensInSystemSCAddress), nil
}

func getAllTokensWithoutSystemAccount(allAddressesTokensMap map[string]map[string]struct{}, systemSCAddress string) map[string]struct{} {
	delete(allAddressesTokensMap, systemSCAddress)

	allTokens := make(map[string]struct{})
	for _, tokens := range allAddressesTokensMap {
		for token := range tokens {
			allTokens[token] = struct{}{}
		}
	}

	return allTokens
}

func getExtraTokens(allTokens, allTokensInSystemSCAddress map[string]struct{}) map[string]struct{} {
	ctTokensOnlyInSystemAcc := 0
	extraTokens := make(map[string]struct{})
	for tokenInSystemSC := range allTokensInSystemSCAddress {
		_, exists := allTokens[tokenInSystemSC]
		if !exists {
			ctTokensOnlyInSystemAcc++
			addTokenInMapIfHasNonce(tokenInSystemSC, extraTokens)
		}
	}

	log.Info("found",
		"num tokens in system account, but not in any other address", ctTokensOnlyInSystemAcc,
		"num of sfts/nfts/metaesdts metadata only found in system sc address", len(extraTokens))

	return extraTokens
}

func addTokenInMapIfHasNonce(token string, tokens map[string]struct{}) {
	if hasNonce(token) {
		tokens[token] = struct{}{}
	}
}

func hasNonce(token string) bool {
	return strings.Count(token, "-") == 2
}

func saveResult(tokens map[string]struct{}, outfile string) error {
	jsonBytes, err := json.MarshalIndent(tokens, "", " ")
	if err != nil {
		return err
	}

	log.Info("writing result in", "file", outfile)
	err = ioutil.WriteFile(outfile, jsonBytes, fs.FileMode(outputFilePerms))
	if err != nil {
		return err
	}

	log.Info("finished exporting zero balance tokens map")
	return nil
}

func crossCheckExtraTokens(extraTokens map[string]struct{}) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	nftGetter := newNFTBalanceGetter(cfg.Config.Gateway.URL)
	elasticClient, err := elastic.NewElasticClient(config.ElasticInstanceConfig{
		URL:      cfg.Config.ElasticIndexerConfig.URL,
		Username: cfg.Config.ElasticIndexerConfig.Username,
		Password: cfg.Config.ElasticIndexerConfig.Password,
	})
	if err != nil {
		return err
	}

	tokensChecker, err := newExtraTokensCrossChecker(elasticClient, nftGetter)
	if err != nil {
		return err
	}

	tokensThatStillExist, err := tokensChecker.crossCheckExtraTokens(extraTokens)
	if err != nil {
		return err
	}

	removeTokensThatStillExist(tokensThatStillExist, extraTokens)
	return nil
}

func loadConfig() (*sysAccConfig.GeneralConfig, error) {
	tomlBytes, err := ioutil.ReadFile(tomlFile)
	if err != nil {
		return nil, err
	}

	var tc sysAccConfig.GeneralConfig
	err = toml.Unmarshal(tomlBytes, &tc)
	if err != nil {
		return nil, err
	}

	return &tc, nil
}

func removeTokensThatStillExist(tokensThatStillExist []string, tokens map[string]struct{}) {
	if len(tokensThatStillExist) == 0 {
		log.Info("all cross-checks were successful; exported tokens are only stored in system account")
		return
	}

	log.Error("found tokens with balances that still exist in other accounts; probably found in pending mbs during snapshot; will remove them from exported tokens",
		"tokens", tokensThatStillExist)

	for _, token := range tokensThatStillExist {
		delete(tokens, token)
	}
}
