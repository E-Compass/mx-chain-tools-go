package main

import (
	"encoding/json"
	"fmt"
	"github.com/ElrondNetwork/elrond-go-core/core/pubkeyConverter"
	logger "github.com/ElrondNetwork/elrond-go-logger"
	elrondFactory "github.com/ElrondNetwork/elrond-go/cmd/node/factory"
	"github.com/ElrondNetwork/elrond-go/common/logging"
	"github.com/ElrondNetwork/elrond-tools-go/trieTools/trieToolsCommon"
	vmcommon "github.com/ElrondNetwork/elrond-vm-common"
	"github.com/urfave/cli"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultLogsPath            = "logs"
	logFilePrefix              = "accounts-tokens-exporter"
	addressLength              = 32
	outputFilePerms            = 0644
	maxIndexerRetrials         = 10
	crossCheckProgressInterval = 1000
)

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

	_, errLogger := attachFileLogger(log, flagsConfig.ContextFlagsConfig)
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

	extraTokens, err := exportZeroTokensBalances(addressTokensMap, flagsConfig.Outfile)
	if err != nil {
		return err
	}

	if flagsConfig.CrossCheck {
		return crossCheckExtraTokens(extraTokens)
	}

	return nil
}

func attachFileLogger(log logger.Logger, flagsConfig trieToolsCommon.ContextFlagsConfig) (elrondFactory.FileLoggingHandler, error) {
	var fileLogging elrondFactory.FileLoggingHandler
	var err error
	if flagsConfig.SaveLogFile {
		fileLogging, err = logging.NewFileLogging(logging.ArgsFileLogging{
			WorkingDir:      flagsConfig.WorkingDir,
			DefaultLogsPath: defaultLogsPath,
			LogFilePrefix:   logFilePrefix,
		})
		if err != nil {
			return nil, fmt.Errorf("%w creating a log file", err)
		}
	}

	err = logger.SetDisplayByteSlice(logger.ToHex)
	log.LogIfError(err)
	logger.ToggleLoggerName(flagsConfig.EnableLogName)
	logLevelFlagValue := flagsConfig.LogLevel
	err = logger.SetLogLevel(logLevelFlagValue)
	if err != nil {
		return nil, err
	}

	if flagsConfig.DisableAnsiColor {
		err = logger.RemoveLogObserver(os.Stdout)
		if err != nil {
			return nil, err
		}

		err = logger.AddLogObserver(os.Stdout, &logger.PlainFormatter{})
		if err != nil {
			return nil, err
		}
	}
	log.Trace("logger updated", "level", logLevelFlagValue, "disable ANSI color", flagsConfig.DisableAnsiColor)

	return fileLogging, nil
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
			for tokenInSrc := range tokensSrc {
				dest[addressSrc][tokenInSrc] = struct{}{}
			}
		}
	}
}

func exportZeroTokensBalances(allAddressesTokensMap map[string]map[string]struct{}, outfile string) (map[string]struct{}, error) {
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

	ctTokensInSystemAccButNotInOtherAddress := 0
	tokensToDelete := make(map[string]struct{})
	for tokenInSystemSC := range allTokensInSystemSCAddress {
		_, exists := allTokens[tokenInSystemSC]
		if !exists {

			ctTokensInSystemAccButNotInOtherAddress++
			if strings.Count(tokenInSystemSC, "-") == 2 {
				tokensToDelete[tokenInSystemSC] = struct{}{}
			}
		}
	}

	log.Info("found",
		"num tokens in system account, but not in any other address", ctTokensInSystemAccButNotInOtherAddress,
		"num of sfts/nfts/metaesdts metadata only found in system sc address", len(tokensToDelete),
	)
	err = saveResult(tokensToDelete, outfile)
	if err != nil {
		return nil, err
	}

	return tokensToDelete, nil
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

type indexerResp struct {
	Count uint64 `json:"count"`
}

func crossCheckExtraTokens(tokens map[string]struct{}) error {
	numTokens := len(tokens)
	log.Info("starting to cross-check", "num of tokens", numTokens)

	numTokensCrossChecked := 0
	for token := range tokens {
		resp, err := getResponseWithRetrial(token)
		if err != nil {
			log.Error("failed to cross check", "token", token, "error", err)
			continue
		}

		decoder := json.NewDecoder(resp.Body)
		response := indexerResp{}
		err = decoder.Decode(&response)
		if err != nil {
			log.Error("failed to decode body response",
				"token", token,
				"error", err,
				"response body", getBody(resp),
				"response status", resp.Status)
			continue
		}

		if response.Count != 0 {
			log.Error("cross-check failed",
				"token", token,
				"actual num of addresses holding the token", response.Count)
		}

		numTokensCrossChecked++
		if numTokensCrossChecked%crossCheckProgressInterval == 0 {
			go printProgress(numTokens, numTokensCrossChecked)
		}

		time.Sleep(40 * time.Millisecond)
	}

	log.Info("finished cross-checking")
	return nil
}

func printProgress(numTokens, numTokensCrossChecked int) {
	log.Info("status",
		"num cross checked tokens", numTokensCrossChecked,
		"remaining num of tokens to check", numTokens-numTokensCrossChecked,
		"progress(%)", (100*numTokensCrossChecked)/numTokens) // this should not panic with div by zero, since func is only called if numTokens > 0
}

func getResponseWithRetrial(token string) (*http.Response, error) {
	ctRetrials := 0
	for ctRetrials < maxIndexerRetrials {
		resp, err := http.Get("https://index.elrond.com/accountsesdt/_count?default_operator=AND&q=identifier:" + token)
		if err == nil {
			return resp, nil
		}

		log.Warn("could not get http response",
			"token", token,
			"error", err,
			"response body", getBody(resp),
			"num retrials", ctRetrials)

		ctRetrials++
	}

	return nil, fmt.Errorf("could not get indexer status for token = %s after num of retrials = %d", token, maxIndexerRetrials)
}

func getBody(response *http.Response) string {
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		log.Error("could not ready bytes from body", "error", err)
		return ""
	}

	return string(bodyBytes)
}
