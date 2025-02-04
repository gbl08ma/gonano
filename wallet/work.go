package wallet

import (
	"encoding/hex"

	"github.com/hectorchu/gonano/pow"
)

func (w *Wallet) workGenerate(data []byte) (work []byte, err error) {
	difficulty, _ := hex.DecodeString(w.WorkDifficulty)
	if work, _, _, err = w.RPCWork.WorkGenerate(data, difficulty); err == nil {
		return
	}
	return pow.Generate(data, difficulty)
}

func (w *Wallet) workGenerateReceive(data []byte) (work []byte, err error) {
	difficulty, _ := hex.DecodeString(w.ReceiveWorkDifficulty)
	if work, _, _, err = w.RPCWork.WorkGenerate(data, difficulty); err == nil {
		return
	}
	return pow.Generate(data, difficulty)
}
