package stellar

import (
	"context"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
)

const stellarPageLimit = 100

// GetHorizonClient gets the horizon client based on the wallet's network
func GetHorizonClient(network string) (*horizonclient.Client, error) {
	switch network {
	case "testnet":
		return horizonclient.DefaultTestNetClient, nil
	case "production":
		return horizonclient.DefaultPublicNetClient, nil
	default:
		return nil, errors.New("network is not supported")
	}
}

func FetchTransactions(ctx context.Context, client *horizonclient.Client, address string, cursor string, handler func(op horizon.Transaction)) error {

	opRequest := horizonclient.TransactionRequest{
		ForAccount:    address,
		IncludeFailed: false,
		Cursor:        cursor,
		Limit:         stellarPageLimit,
	}

	for {
		if ctx.Err() != nil {
			return nil
		}

		response, err := client.Transactions(opRequest)
		if err != nil {
			log.Info("Error getting transactions for stellar account", "address", opRequest.ForAccount, "cursor", opRequest.Cursor, "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
				continue
			}

		}
		for _, tx := range response.Embedded.Records {
			handler(tx)
			opRequest.Cursor = tx.PagingToken()
		}
		if len(response.Embedded.Records) == 0 {
			return nil
		}

	}

}
