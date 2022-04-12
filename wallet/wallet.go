package wallet

import (
	"math/big"
	"sync"

	"github.com/hectorchu/gonano/rpc"
	"github.com/hectorchu/gonano/util"
)

// Wallet represents a wallet.
type Wallet struct {
	isBanano              bool
	seed                  []byte
	isBip39               bool
	nextIndex             uint32
	accounts              map[string]*Account
	accountsMutex         sync.RWMutex
	RPC, RPCWork          rpc.Client
	WorkDifficulty        string
	ReceiveWorkDifficulty string
	impl                  interface {
		deriveAccount(*Account) error
		signBlock(*Account, *rpc.Block) error
	}
}

// NewWallet creates a new wallet.
func NewWallet(seed []byte) (w *Wallet, err error) {
	w = newWallet(seed, false)
	return
}

// NewBananoWallet creates a new Banano wallet.
func NewBananoWallet(seed []byte) (w *Wallet, err error) {
	w = newWallet(seed, true)
	return
}

// NewBip39Wallet creates a new BIP39 wallet.
func NewBip39Wallet(mnemonic, password string) (w *Wallet, err error) {
	seed, err := newBip39Seed(mnemonic, password)
	if err != nil {
		return
	}
	w = newWallet(seed, false)
	w.isBip39 = true
	return
}

// NewBip39BananoWallet creates a new BIP39 Banano wallet.
func NewBip39BananoWallet(mnemonic, password string) (w *Wallet, err error) {
	seed, err := newBip39Seed(mnemonic, password)
	if err != nil {
		return
	}
	w = newWallet(seed, true)
	w.isBip39 = true
	return
}

// NewLedgerWallet creates a new Ledger wallet.
func NewLedgerWallet() (w *Wallet, err error) {
	w = newWallet(nil, false)
	w.impl = ledgerImpl{}
	return
}

func newWallet(seed []byte, isBanano bool) *Wallet {
	w := &Wallet{
		isBanano:              isBanano,
		seed:                  seed,
		accounts:              make(map[string]*Account),
		RPC:                   rpc.Client{URL: "https://mynano.ninja/api/node"},
		RPCWork:               rpc.Client{URL: "http://[::1]:7076"},
		impl:                  seedImpl{},
		WorkDifficulty:        "fffffff800000000",
		ReceiveWorkDifficulty: "fffffe0000000000",
	}
	if isBanano {
		w.RPC = rpc.Client{URL: "https://api-beta.banano.cc"}
	}
	return w
}

// ScanForAccounts scans for accounts.
func (w *Wallet) ScanForAccounts() (err error) {
	accounts := make([]string, 10)
	for i := range accounts {
		a, err := w.NewAccount(nil)
		if err != nil {
			return err
		}
		accounts[i] = a.Address()
	}
	balances, err := w.RPC.AccountsBalances(accounts)
	if err != nil {
		return
	}
	frontiers, err := w.RPC.AccountsFrontiers(accounts)
	if err != nil {
		return
	}
	i := len(accounts) - 1
	for ; i >= 0; i-- {
		if balances[accounts[i]].Pending.Sign() > 0 {
			break
		}
		if frontiers[accounts[i]] != nil {
			break
		}
		func() {
			w.accountsMutex.Lock()
			defer w.accountsMutex.Unlock()
			w.nextIndex = w.accounts[accounts[i]].index
			delete(w.accounts, accounts[i])
		}()
	}
	if i < 5 {
		return
	}
	return w.ScanForAccounts()
}

// NewAccount creates a new account.
func (w *Wallet) NewAccount(index *uint32) (a *Account, err error) {
	index2 := w.nextIndex
	if index != nil {
		index2 = *index
	}
	a = &Account{w: w, index: index2}
	if err = w.impl.deriveAccount(a); err != nil {
		return
	}
	pubkeyToAddress := util.PubkeyToAddress
	if w.isBanano {
		pubkeyToAddress = util.PubkeyToBananoAddress
	}
	if a.address, err = pubkeyToAddress(a.pubkey); err != nil {
		return
	}
	if index == nil {
		w.nextIndex++
	}

	done := false
	func() {
		w.accountsMutex.Lock()
		defer w.accountsMutex.Unlock()
		if _, ok := w.accounts[a.address]; !ok {
			w.accounts[a.address] = a
			done = true
		}
	}()

	if !done && index == nil {
		return w.NewAccount(nil)
	}
	return
}

// GetAccount gets the account with address or nil if not found.
func (w *Wallet) GetAccount(address string) *Account {
	w.accountsMutex.RLock()
	defer w.accountsMutex.RUnlock()
	return w.accounts[address]
}

// GetAccounts gets all the accounts in the wallet.
func (w *Wallet) GetAccounts() (accounts []*Account) {
	w.accountsMutex.RLock()
	defer w.accountsMutex.RUnlock()

	accounts = make([]*Account, 0, len(w.accounts))
	for _, account := range w.accounts {
		accounts = append(accounts, account)
	}
	return
}

// ReceivePendings pockets all pending amounts.
func (w *Wallet) ReceivePendings(threshold *big.Int) (err error) {

	var accounts []string
	accountsMapCopy := make(map[string]*Account)
	func() {
		w.accountsMutex.RLock()
		defer w.accountsMutex.RUnlock()
		accounts = make([]string, 0, len(w.accounts))
		for address, account := range w.accounts {
			accounts = append(accounts, address)
			accountsMapCopy[address] = account
		}
	}()
	pendings, err := w.RPC.AccountsPending(accounts, -1, &rpc.RawAmount{Int: *threshold})
	if err != nil {
		return
	}
	for account, pendings := range pendings {
		if err = accountsMapCopy[account].receivePendings(pendings); err != nil {
			return
		}
	}
	return
}
