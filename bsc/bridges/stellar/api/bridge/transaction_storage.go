package bridge

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/stellar/go/clients/horizonclient"
	hProtocol "github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
)

type StellarTransactionStorage struct {
	network               string
	addressToScan         string
	knownTransactionMemos map[string]struct{}
	stellarCursor         string
}

func NewStellarTransactionStorage(network, addressToScan string) *StellarTransactionStorage {
	return &StellarTransactionStorage{
		network:               network,
		addressToScan:         addressToScan,
		knownTransactionMemos: make(map[string]struct{}),
	}
}

func (s *StellarTransactionStorage) CheckForExistingTransactionHash(txn *txnbuild.Transaction) error {
	log.Info("checking tx hash")
	txMemo, err := txn.Memo().ToXDR()
	if err != nil {
		return err
	}

	// only check transaction with hash memos
	if txMemo.Type != xdr.MemoTypeMemoHash {
		return nil
	}

	hashMemo := txn.Memo().(txnbuild.MemoHash)
	txMemoString := hex.EncodeToString(hashMemo[:])

	_, ok := s.knownTransactionMemos[txMemoString]
	if ok {
		return fmt.Errorf("transaction with memo %s already exists on bridge account %s", txMemoString, txn.SourceAccount().AccountID)
	}

	// trigger a rescan
	// will not rescan from start since we saved the cursor
	err = s.ScanBridgeAccount()
	if err != nil {
		return err
	}

	_, ok = s.knownTransactionMemos[txMemoString]
	if ok {
		return fmt.Errorf("transaction with memo %s already exists on bridge account %s", txMemoString, txn.SourceAccount().AccountID)
	}
	log.Info("transaction not found")

	return nil
}

func (s *StellarTransactionStorage) ScanBridgeAccount() error {
	if s.addressToScan == "" {
		return errors.New("no master bridge account set, aborting now")
	}

	transactionHandler := func(tx hProtocol.Transaction) {
		if tx.MemoType != "hash" && tx.MemoType != "return" {
			return
		}

		bytes, err := base64.StdEncoding.DecodeString(tx.Memo)
		if err != nil {
			return
		}
		memoAsHex := hex.EncodeToString(bytes)

		_, ok := s.knownTransactionMemos[memoAsHex]
		if !ok {
			log.Info("storing memo hash in known transaction storage", "hash", memoAsHex)
			// add the transaction memo to the list of known transaction memos
			s.knownTransactionMemos[memoAsHex] = struct{}{}
		}
	}

	err := s.FetchTransactions(context.Background(), s.stellarCursor, transactionHandler)
	if err != nil {
		return err
	}

	return nil
}

func (s *StellarTransactionStorage) FetchTransactions(ctx context.Context, cursor string, handler func(op hProtocol.Transaction)) error {
	client, err := s.getHorizonClient()
	if err != nil {
		return err
	}

	opRequest := horizonclient.TransactionRequest{
		ForAccount:    s.addressToScan,
		IncludeFailed: false,
		Cursor:        s.stellarCursor,
		Limit:         stellarPageLimit,
	}
	log.Info("Start fetching stellar transactions", "horizon", client.HorizonURL, "account", opRequest.ForAccount, "cursor", opRequest.Cursor)

	for {
		if ctx.Err() != nil {
			return nil
		}

		response, err := client.Transactions(opRequest)
		if err != nil {
			log.Info("Error getting transactions for stellar account", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
				continue
			}

		}
		for _, tx := range response.Embedded.Records {
			handler(tx)
			s.stellarCursor = tx.PagingToken()
			opRequest.Cursor = s.stellarCursor
		}
		if len(response.Embedded.Records) == 0 {
			return nil
		}

	}

}

// GetHorizonClient gets the horizon client based on the transaction storage's network
func (s *StellarTransactionStorage) getHorizonClient() (*horizonclient.Client, error) {
	switch s.network {
	case "testnet":
		return horizonclient.DefaultTestNetClient, nil
	case "production":
		return horizonclient.DefaultPublicNetClient, nil
	default:
		return nil, errors.New("network is not supported")
	}
}
