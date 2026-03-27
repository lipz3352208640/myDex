package entity

import (
	"encoding/json"
)

type WsSub struct {
	Id      int    `json:"id"`
	JsonRpc string `json:"jsonrpc"`
	Method  string `json:"method"`
}

type WsSubAck struct {
	Jsonrpc string `json:"jsonrpc"`
	Id      int    `json:"id"`
	Result  int64  `json:"result"`
}

type WsResp struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		Result struct {
			Slot   uint64 `json:"slot"`
			Parent uint64 `json:"parent"`
			Root   uint64 `json:"root"`
		} `json:"result"`
		Subscription int `json:"subscription"`
	} `json:"params"`
}

type WsErrResp struct {
	Jsonrpc string `json:"jsonrpc"`
	Id      int    `json:"id"`
	Error   struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"`
	} `json:"error"`
}

type WsUnsub struct {
	Id      int     `json:"id"`
	JsonRpc string  `json:"jsonrpc"`
	Method  string  `json:"method"`
	Params  []int64 `json:"params"`
}

func (ws *WsUnsub) CancleWsSub(subscriptionID int64) []byte {
	ws.Id = 1
	ws.JsonRpc = "2.0"
	ws.Method = "slotUnsubscribe"
	ws.Params = []int64{subscriptionID}

	jsonStr, _ := json.Marshal(ws)
	return jsonStr
}

func (ws *WsSub) ApplyWsSub() []byte {
	ws.Id = 1
	ws.JsonRpc = "2.0"
	ws.Method = "slotSubscribe"

	jsonStr, _ := json.Marshal(ws)
	return jsonStr
}
