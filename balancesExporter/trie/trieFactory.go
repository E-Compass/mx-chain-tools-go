package trie

import (
	"github.com/ElrondNetwork/elrond-go-core/hashing/blake2b"
	"github.com/ElrondNetwork/elrond-go-core/marshal"
	"github.com/ElrondNetwork/elrond-go/epochStart/notifier"
	"github.com/ElrondNetwork/elrond-go/sharding"
	"github.com/ElrondNetwork/elrond-go/storage/pruning"
	"github.com/ElrondNetwork/elrond-go/testscommon"
	"github.com/ElrondNetwork/elrond-go/trie"

	storageFactory "github.com/ElrondNetwork/elrond-go/storage/factory"
)

const (
	maxTrieLevelInMemory = 5
	maxBatchSize         = 45000
)

var (
	hasher     = blake2b.NewBlake2b()
	marshaller = &marshal.GogoProtoMarshalizer{}
)

type ArgsNewTrieFactory struct {
	ShardCoordinator sharding.Coordinator
	DbPath           string
	Epoch            uint32
}

type trieFactory struct {
	shardCoordinator sharding.Coordinator
	dbPath           string
	epoch            uint32
}

func NewTrieFactory(args ArgsNewTrieFactory) *trieFactory {
	return &trieFactory{
		shardCoordinator: args.ShardCoordinator,
		dbPath:           args.DbPath,
		epoch:            args.Epoch,
	}
}

func (factory *trieFactory) CreateTrie() (*trieWrapper, error) {
	cacheConfig := getCacheConfig()
	dbConfig := getDbConfig(factory.dbPath)
	pathManager := newSimplePathManager(factory.dbPath)

	args := &pruning.StorerArgs{
		Identifier:                "",
		ShardCoordinator:          factory.shardCoordinator,
		CacheConf:                 cacheConfig,
		PathManager:               pathManager,
		DbPath:                    "",
		PersisterFactory:          storageFactory.NewPersisterFactory(dbConfig),
		Notifier:                  notifier.NewManualEpochStartNotifier(),
		OldDataCleanerProvider:    &testscommon.OldDataCleanerProviderStub{},
		CustomDatabaseRemover:     &testscommon.CustomDatabaseRemoverStub{},
		MaxBatchSize:              maxBatchSize,
		NumOfEpochsToKeep:         factory.epoch + 1,
		NumOfActivePersisters:     factory.epoch + 1,
		StartingEpoch:             factory.epoch,
		PruningEnabled:            true,
		EnabledDbLookupExtensions: false,
	}

	db, err := pruning.NewTriePruningStorer(args)
	if err != nil {
		return nil, err
	}

	storageManager, err := trie.NewTrieStorageManagerWithoutPruning(db)
	if err != nil {
		return nil, err
	}

	t, err := trie.NewTrie(storageManager, marshaller, hasher, maxTrieLevelInMemory)
	if err != nil {
		return nil, err
	}

	return newTrieWrapper(t), nil
}
