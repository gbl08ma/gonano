package wallet

import (
	"errors"

	"github.com/hectorchu/gonano/rpc"
	"github.com/hectorchu/gonano/wallet/ed25519"
)

type seedImpl struct{}

func (seedImpl) deriveAccount(a *Account) (err error) {
	var key []byte
	if a.w.isBip39 {
		key, err = deriveBip39Key(a.w.seed, a.index)
	} else {
		key, err = deriveKey(a.w.seed, a.index)
	}
	if err != nil {
		return
	}
	a.pubkey, a.key, err = deriveKeypair(key)
	return
}

func (seedImpl) signBlock(a *Account, block *rpc.Block) (err error) {
	hash, err := block.Hash()
	if err != nil {
		return
	}
	block.Signature = ed25519.Sign(a.key, hash)
	return
}

type ledgerImpl struct{}

func (ledgerImpl) deriveAccount(a *Account) (err error) {
	return errors.New("ledger support not available")
}

func (ledgerImpl) signBlock(a *Account, block *rpc.Block) (err error) {
	return errors.New("ledger support not available")
}
