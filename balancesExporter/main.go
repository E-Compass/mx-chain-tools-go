package main

import (
	"os"

	"github.com/ElrondNetwork/elrond-go-core/core"
	"github.com/ElrondNetwork/elrond-go/sharding"
	"github.com/ElrondNetwork/elrond-tools-go/balancesExporter/blocks"
	"github.com/ElrondNetwork/elrond-tools-go/balancesExporter/export"
	"github.com/ElrondNetwork/elrond-tools-go/balancesExporter/trie"
	"github.com/urfave/cli"
)

const (
	rootHashLength = 32
	addressLength  = 32
)

func main() {
	app := cli.NewApp()
	app.Name = "Balances exporter CLI app"
	app.Usage = "Tool for exporting balances of accounts (given a node db)"
	app.Flags = getAllCliFlags()
	app.Authors = []cli.Author{
		{
			Name:  "The Elrond Team",
			Email: "contact@elrond.com",
		},
	}

	app.Action = startExport

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}

func startExport(ctx *cli.Context) error {
	cliFlags := getParsedCliFlags(ctx)

	fileLogging, err := initializeLogger(cliFlags.logLevel)
	if err != nil {
		return err
	}
	defer func() {
		_ = fileLogging.Close()
	}()

	actualShardCoordinator, projectedShardCoordinator, err := createShardCoordinators(cliFlags)
	if err != nil {
		return err
	}

	trieFactory := trie.NewTrieFactory(trie.ArgsNewTrieFactory{
		ShardCoordinator: actualShardCoordinator,
		DbPath:           cliFlags.dbPath,
		Epoch:            cliFlags.epoch,
	})

	trieWrapper, err := trieFactory.CreateTrie()
	if err != nil {
		return err
	}
	defer trieWrapper.Close()

	blocksRepository := blocks.NewBlocksRepository(blocks.ArgsNewBlocksRepository{
		DbPath:      cliFlags.dbPath,
		Epoch:       cliFlags.epoch,
		Shard:       cliFlags.shard,
		TrieWrapper: trieWrapper,
	})

	bestBlock, err := blocksRepository.FindBestBlock()
	if err != nil {
		return err
	}

	exporter := export.NewExporter(export.ArgsNewExporter{
		ShardCoordinator: projectedShardCoordinator,
		TrieWrapper:      trieWrapper,
		Format:           cliFlags.exportFormat,
		Currency:         cliFlags.currency,
		CurrencyDecimals: cliFlags.currencyDecimals,
		WithContracts:    cliFlags.withContracts,
		WithZero:         cliFlags.withZero,
	})

	err = exporter.ExportBalancesAtBlock(bestBlock)
	if err != nil {
		return err
	}

	return nil
}

func createShardCoordinators(cliFlags parsedCliFlags) (sharding.Coordinator, sharding.Coordinator, error) {
	actualShardCoordinator, err := sharding.NewMultiShardCoordinator(cliFlags.numShards, cliFlags.shard)
	if err != nil {
		return nil, nil, err
	}

	projectedShardCoordinator, err := sharding.NewMultiShardCoordinator(core.MaxNumShards, cliFlags.projectedShard)
	if err != nil {
		return nil, nil, err
	}

	if cliFlags.projectedShardIsSet {
		return actualShardCoordinator, projectedShardCoordinator, nil
	}

	return actualShardCoordinator, actualShardCoordinator, nil
}
