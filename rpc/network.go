package rpc

import (
	"encoding/json"
	"strconv"
)

// ActiveDifficulty returns the difficulty values (16 hexadecimal digits string, 64 bit)
// for the minimum required on the network (network_minimum) as well as the current active
// difficulty seen on the network (network_current, 10 second trended average of adjusted
// difficulty seen on prioritized transactions) which can be used to perform rework for
// better prioritization of transaction processing. A multiplier of the network_current
// from the base difficulty of network_minimum is also provided for comparison.
// network_receive_minimum and network_receive_current are also provided as lower
// thresholds exclusively for receive blocks.
func (c *Client) ActiveDifficulty() (
	multiplier float64,
	networkCurrent, networkMinimum,
	networkReceiveCurrent, networkReceiveMinimum HexData,
	difficultyTrend []float64,
	err error,
) {
	resp, err := c.send(map[string]interface{}{"action": "active_difficulty", "include_trend": true})
	if err != nil {
		return
	}
	var v struct {
		Multiplier            float64  `json:"multiplier,string"`
		NetworkCurrent        HexData  `json:"network_current"`
		NetworkMinimum        HexData  `json:"network_minimum"`
		NetworkReceiveCurrent HexData  `json:"network_receive_current"`
		NetworkReceiveMinimum HexData  `json:"network_receive_minimum"`
		DifficultyTrend       []string `json:"difficulty_trend"`
	}
	if err = json.Unmarshal(resp, &v); err != nil {
		// v22.1 node has a bug where DifficultyTrend will be the empty string
		// a fix for the node is here (this RPC is deprecated) https://github.com/nanocurrency/nano-node/pull/3343/files
		// but that will only come with v23
		// in the meantime, attempt to do the backwards-compatibility ourselves
		var v2 struct {
			Multiplier            float64 `json:"multiplier,string"`
			NetworkCurrent        HexData `json:"network_current"`
			NetworkMinimum        HexData `json:"network_minimum"`
			NetworkReceiveCurrent HexData `json:"network_receive_current"`
			NetworkReceiveMinimum HexData `json:"network_receive_minimum"`
		}
		if err = json.Unmarshal(resp, &v2); err != nil {
			return
		}
		v.Multiplier = v2.Multiplier
		v.NetworkCurrent = v2.NetworkCurrent
		v.NetworkMinimum = v2.NetworkMinimum
		v.NetworkReceiveCurrent = v2.NetworkReceiveCurrent
		v.NetworkReceiveMinimum = v2.NetworkReceiveMinimum
		v.DifficultyTrend = []string{"1.000000000000000"}
	}
	difficultyTrend = make([]float64, len(v.DifficultyTrend))
	for i, s := range v.DifficultyTrend {
		if difficultyTrend[i], err = strconv.ParseFloat(s, 64); err != nil {
			return
		}
	}
	return v.Multiplier,
		v.NetworkCurrent, v.NetworkMinimum,
		v.NetworkReceiveCurrent, v.NetworkReceiveMinimum,
		difficultyTrend, err
}

// AvailableSupply returns how many raw are in the public supply.
func (c *Client) AvailableSupply() (available *RawAmount, err error) {
	resp, err := c.send(map[string]interface{}{"action": "available_supply"})
	if err != nil {
		return
	}
	var v struct{ Available *RawAmount }
	err = json.Unmarshal(resp, &v)
	return v.Available, err
}
