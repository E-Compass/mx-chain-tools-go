package mocks

import (
	"context"

	"github.com/ElrondNetwork/elrond-sdk-erdgo/core"
	"github.com/ElrondNetwork/elrond-sdk-erdgo/data"
)

// ProxyStub -
type ProxyStub struct {
	GetNetworkConfigCalled               func(ctx context.Context) (*data.NetworkConfig, error)
	GetDefaultTransactionArgumentsCalled func(
		ctx context.Context,
		address core.AddressHandler,
		networkConfigs *data.NetworkConfig,
	) (data.ArgCreateTransaction, error)
}

// GetNetworkConfig -
func (ps *ProxyStub) GetNetworkConfig(ctx context.Context) (*data.NetworkConfig, error) {
	if ps.GetNetworkConfigCalled != nil {
		return ps.GetNetworkConfigCalled(ctx)
	}

	return nil, nil
}

// GetDefaultTransactionArguments -
func (ps *ProxyStub) GetDefaultTransactionArguments(
	ctx context.Context,
	address core.AddressHandler,
	networkConfigs *data.NetworkConfig,
) (data.ArgCreateTransaction, error) {
	if ps.GetDefaultTransactionArgumentsCalled != nil {
		return ps.GetDefaultTransactionArgumentsCalled(ctx, address, networkConfigs)
	}

	return data.ArgCreateTransaction{}, nil
}
