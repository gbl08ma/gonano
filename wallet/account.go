package wallet

import (
	"encoding/hex"
	"errors"
	"math/big"

	"github.com/hectorchu/gonano/rpc"
	"github.com/hectorchu/gonano/util"
)

// Account represents a wallet account.
type Account struct {
	w              *Wallet
	index          uint32
	key, pubkey    []byte
	address        string
	representative string
}

// Address returns the address of the account.
func (a *Account) Address() string {
	return a.address
}

// Index returns the derivation index of the account.
func (a *Account) Index() uint32 {
	return a.index
}

// Balance gets the confirmed and pending balances for account.
func (a *Account) Balance() (balance, pending *big.Int, err error) {
	b, p, err := a.w.RPC.AccountBalance(a.address)
	if err != nil {
		return
	}
	return &b.Int, &p.Int, nil
}

// Send sends an amount to an account.
func (a *Account) Send(account string, amount *big.Int) (hash rpc.BlockHash, err error) {
	block, err := a.SendBlock(account, amount)
	if err != nil {
		return
	}
	if block.Work, err = a.w.workGenerate(block.Previous); err != nil {
		return
	}
	return a.w.RPC.Process(block, "send")
}

// SendBlock generates a signed send block.
func (a *Account) SendBlock(account string, amount *big.Int) (block *rpc.Block, err error) {
	link, err := util.AddressToPubkey(account)
	if err != nil {
		return
	}
	info, err := a.w.RPC.AccountInfo(a.address)
	if err != nil {
		return
	}
	if a.representative == "" {
		a.representative = info.Representative
	}
	if info.Balance.Sub(&info.Balance.Int, amount).Sign() < 0 {
		return nil, errors.New("insufficient funds")
	}
	block = &rpc.Block{
		Type:           "state",
		Account:        a.address,
		Previous:       info.Frontier,
		Representative: a.representative,
		Balance:        info.Balance,
		Link:           link,
	}
	return block, a.w.impl.signBlock(a, block)
}

// SendDestination is a destination for a send block
type SendDestination struct {
	Account string
	Amount  *big.Int
}

// Send sends multiple amounts to multiple accounts. The caller must guarantee that no new blocks are created for this account until this function returns
func (a *Account) SendMultiple(destinations []SendDestination) (hashes []rpc.BlockHash, err error) {
	blocks, err := a.SendBlocks(destinations)
	if err != nil {
		return
	}
	blocksWithWorkChan := make(chan *rpc.Block, len(destinations))
	errChan := make(chan error)
	go func() {
		for i := range blocks {
			if blocks[i].Work, err = a.w.workGenerate(blocks[i].Previous); err != nil {
				errChan <- err
				return
			}
			blocksWithWorkChan <- blocks[i]
		}
		close(blocksWithWorkChan)
	}()

	for {
		select {
		case block, ok := <-blocksWithWorkChan:
			if !ok {
				return hashes, nil
			}
			hash, err := a.w.RPC.Process(block, "send")
			if err != nil {
				return nil, err
			}
			hashes = append(hashes, hash)
		case err := <-errChan:
			return nil, err
		}
	}
}

// SendBlocks generates multiple signed send blocks. The caller must guarantee that no new blocks are created for this account between the generated blocks
func (a *Account) SendBlocks(destinations []SendDestination) ([]*rpc.Block, error) {
	blocks := []*rpc.Block{}

	info, err := a.w.RPC.AccountInfo(a.address)
	if err != nil {
		return nil, err
	}

	frontier := info.Frontier
	for _, destination := range destinations {
		link, err := util.AddressToPubkey(destination.Account)
		if err != nil {
			return nil, err
		}

		if a.representative == "" {
			a.representative = info.Representative
		}
		if info.Balance.Sub(&info.Balance.Int, destination.Amount).Sign() < 0 {
			return nil, errors.New("insufficient funds")
		}
		block := &rpc.Block{
			Type:           "state",
			Account:        a.address,
			Previous:       frontier,
			Representative: a.representative,
			Balance:        &rpc.RawAmount{Int: *new(big.Int).Set(&info.Balance.Int)},
			Link:           link,
		}
		err = a.w.impl.signBlock(a, block)
		if err != nil {
			return []*rpc.Block{}, err
		}
		blocks = append(blocks, block)
		frontier, err = block.Hash()
		if err != nil {
			return nil, err
		}
	}

	return blocks, nil
}

// ReceivePendings pockets all pending amounts.
func (a *Account) ReceivePendings(threshold *big.Int) (err error) {
	pendings, err := a.w.RPC.AccountsPending([]string{a.address}, -1,
		&rpc.RawAmount{Int: *threshold})
	if err != nil {
		return
	}
	return a.receivePendings(pendings[a.address])
}

// ReceiveAndReturnPendings pockets all pending amounts and returns the list of sources.
func (a *Account) ReceiveAndReturnPendings(threshold *big.Int) (receivedPendings rpc.HashToPendingMap, err error) {
	pendings, err := a.w.RPC.AccountsPending([]string{a.address}, -1,
		&rpc.RawAmount{Int: *threshold})
	if err != nil {
		return
	}
	receivedPendings = pendings[a.address]
	err = a.receivePendings(receivedPendings)
	return
}

// ReceivePending pockets the specified link block.
func (a *Account) ReceivePending(link rpc.BlockHash) (hash rpc.BlockHash, err error) {
	info, err := a.w.RPC.AccountInfo(a.address)
	if err != nil {
		info.Balance = &rpc.RawAmount{}
	}
	block, err := a.w.RPC.BlockInfo(link)
	if err != nil {
		return
	}
	info.Balance.Add(&info.Balance.Int, &block.Amount.Int)
	return a.receivePending(info, link)
}

func (a *Account) receivePendings(pendings rpc.HashToPendingMap) (err error) {
	if len(pendings) == 0 {
		return
	}
	info, err := a.w.RPC.AccountInfo(a.address)
	if err != nil {
		info.Balance = &rpc.RawAmount{}
	}
	for hash, pending := range pendings {
		var link rpc.BlockHash
		if link, err = hex.DecodeString(hash); err != nil {
			return
		}
		info.Balance.Add(&info.Balance.Int, &pending.Amount.Int)
		if info.Frontier, err = a.receivePending(info, link); err != nil {
			return
		}
	}
	return
}

func (a *Account) receivePending(info rpc.AccountInfo, link rpc.BlockHash) (hash rpc.BlockHash, err error) {
	workHash := info.Frontier
	if info.Frontier == nil {
		info.Frontier = make(rpc.BlockHash, 32)
		workHash = a.pubkey
	}
	if a.representative == "" {
		a.representative = info.Representative
		if a.representative == "" {
			a.representative = "nano_3gonano8jnse4zm65jaiki9tk8ry4jtgc1smarinukho6fmbc45k3icsh6en"
		}
	}
	block := &rpc.Block{
		Type:           "state",
		Account:        a.address,
		Previous:       info.Frontier,
		Representative: a.representative,
		Balance:        info.Balance,
		Link:           link,
	}
	if err = a.w.impl.signBlock(a, block); err != nil {
		return
	}
	if block.Work, err = a.w.workGenerateReceive(workHash); err != nil {
		return
	}
	return a.w.RPC.Process(block, "receive")
}

// SetRep sets the account's representative for future blocks.
func (a *Account) SetRep(representative string) (err error) {
	if _, err = util.AddressToPubkey(representative); err != nil {
		return
	}
	a.representative = representative
	return
}

// ChangeRep changes the account's representative.
func (a *Account) ChangeRep(representative string) (hash rpc.BlockHash, err error) {
	info, err := a.w.RPC.AccountInfo(a.address)
	if err != nil {
		return
	}
	block := &rpc.Block{
		Type:           "state",
		Account:        a.address,
		Previous:       info.Frontier,
		Representative: representative,
		Balance:        info.Balance,
		Link:           make(rpc.BlockHash, 32),
	}
	if err = a.w.impl.signBlock(a, block); err != nil {
		return
	}
	if block.Work, err = a.w.workGenerate(info.Frontier); err != nil {
		return
	}
	if hash, err = a.w.RPC.Process(block, "change"); err == nil {
		a.representative = representative
	}
	return
}
