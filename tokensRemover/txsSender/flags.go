package main

import (
	"github.com/ElrondNetwork/elrond-tools-go/tokensRemover/metaDataRemover/config"
	"github.com/ElrondNetwork/elrond-tools-go/trieTools/trieToolsCommon"
	"github.com/urfave/cli"
)

var (
	tokens = cli.StringFlag{
		Name:  "tokens",
		Usage: "This flag specifies the input file; it expects the input to be a map<shardID, tokens>",
		Value: "tokens.json",
	}
	pems = cli.StringFlag{
		Name:  "pem",
		Usage: "This flag specifies pems directory, which should contain multiple pems to be used to sign txs. It expects each pem/shardID to be named shard[ID].pem",
		Value: "pems",
	}
)

func getFlags() []cli.Flag {
	return []cli.Flag{
		trieToolsCommon.LogLevel,
		trieToolsCommon.DisableAnsiColor,
		trieToolsCommon.LogSaveFile,
		trieToolsCommon.LogWithLoggerName,
		trieToolsCommon.ProfileMode,
		tokens,
		pems,
	}
}

func getFlagsConfig(ctx *cli.Context) config.ContextFlagsMetaDataRemover {
	flagsConfig := config.ContextFlagsMetaDataRemover{}

	flagsConfig.LogLevel = ctx.GlobalString(trieToolsCommon.LogLevel.Name)
	flagsConfig.SaveLogFile = ctx.GlobalBool(trieToolsCommon.LogSaveFile.Name)
	flagsConfig.EnableLogName = ctx.GlobalBool(trieToolsCommon.LogWithLoggerName.Name)
	flagsConfig.EnablePprof = ctx.GlobalBool(trieToolsCommon.ProfileMode.Name)
	flagsConfig.Tokens = ctx.GlobalString(tokens.Name)
	flagsConfig.Pems = ctx.GlobalString(pems.Name)

	return flagsConfig
}
