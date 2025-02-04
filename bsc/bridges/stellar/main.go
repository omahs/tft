package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/multiformats/go-multiaddr"
	flag "github.com/spf13/pflag"
	"github.com/threefoldfoundation/tft/bsc/bridges/stellar/api/bridge"

	"github.com/ethereum/go-ethereum/log"
)

func main() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))

	var bridgeCfg bridge.BridgeConfig
	flag.StringVar(&bridgeCfg.EthNetworkName, "ethnetwork", "smart-chain-testnet", "eth network name (defines storage directory name)")
	flag.StringVar(&bridgeCfg.EthUrl, "ethurl", "ws://localhost:8576", "ethereum rpc url")
	flag.StringVar(&bridgeCfg.ContractAddress, "contract", "", "smart contract address")
	flag.StringVar(&bridgeCfg.MultisigContractAddress, "mscontract", "", "multisig smart contract address")

	flag.StringVar(&bridgeCfg.Datadir, "datadir", "./storage", "chain data directory")
	flag.StringVar(&bridgeCfg.PersistencyFile, "persistency", "./node.json", "file where last seen blockheight and stellar account cursor is stored")

	flag.StringVar(&bridgeCfg.AccountJSON, "account", "", "ethereum account json")
	flag.StringVar(&bridgeCfg.AccountPass, "password", "", "ethereum account password")

	flag.StringVar(&bridgeCfg.StellarSeed, "secret", "", "stellar secret")
	flag.StringVar(&bridgeCfg.StellarNetwork, "network", "testnet", "stellar network, testnet or production")
	// Fee wallet address where fees are held
	flag.StringVar(&bridgeCfg.StellarFeeWallet, "feewallet", "", "stellar fee wallet address")

	flag.BoolVar(&bridgeCfg.RescanBridgeAccount, "rescan", false, "if true is provided, we rescan the bridge stellar account and mint all transactions again")
	flag.Int64Var(&bridgeCfg.RescanFromHeight, "rescanHeight", 0, "if provided, the bridge will rescan all withdraws from the given height")

	flag.BoolVar(&bridgeCfg.Follower, "follower", false, "if true then the bridge will run in follower mode meaning that it will not submit mint transactions to the multisig contract, if false the bridge will also submit transactions")

	flag.StringVar(&bridgeCfg.BridgeMasterAddress, "master", "", "master stellar public address")
	flag.Int64Var(&bridgeCfg.DepositFee, "depositFee", 50, "sets the depositfee in TFT")

	flag.Parse()

	//TODO cfg.Validate()

	log.Info("connection url provided: ", "url", bridgeCfg.EthUrl)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host, router, err := bridge.NewHost(ctx, bridgeCfg.StellarSeed, bridgeCfg.BridgeMasterAddress)
	if err != nil {
		panic(err)
	}

	ipfs, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ipfs/%s", host.ID().Pretty()))
	if err != nil {
		panic(err)
	}

	for _, addr := range host.Addrs() {
		full := addr.Encapsulate(ipfs)
		log.Info("p2p node address", "address", full.String())
	}

	br, err := bridge.NewBridge(ctx, &bridgeCfg, host, router)
	if err != nil {
		panic(err)
	}

	err = br.Start(ctx)
	if err != nil {
		panic(err)
	}

	if bridgeCfg.Follower {
		signer, err := bridge.NewSignerServer(host, bridgeCfg.StellarConfig, bridgeCfg.BridgeMasterAddress, br.GetBridgeContract())
		if err != nil {
			panic(err)
		}

		// Initially scan bridge account for stellar transactions
		err = signer.StellarTransactionStorage.ScanBridgeAccount()
		if err != nil {
			panic(err)
		}
	}

	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	log.Info("awaiting signal")
	sig := <-sigs
	log.Info("signal", "signal", sig)
	cancel()
	err = br.Close()
	if err != nil {
		panic(err)
	}

	host.Close()
	log.Info("exiting")
	time.Sleep(time.Second * 5)
}
