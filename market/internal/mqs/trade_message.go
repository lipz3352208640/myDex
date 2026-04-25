package mqs

type TradeMessage struct {
	Slot                         int64       `json:"slot"`
	ChainID                      string      `json:"chain_id"`
	ChainIDInt                   int         `json:"chain_id_int"`
	PairAddr                     string      `json:"pair_addr"`
	TxHash                       string      `json:"tx_hash"`
	HashID                       string      `json:"hash_id"`
	Maker                        string      `json:"maker"`
	Type                         string      `json:"type"`
	BaseTokenAmount              float64     `json:"base_token_amount"`
	TokenAmount                  float64     `json:"token_amount"`
	BaseTokenPriceUSD            float64     `json:"base_token_price_usd"`
	TotalUSD                     float64     `json:"total_usd"`
	TokenPriceUSD                float64     `json:"token_price_usd"`
	To                           string      `json:"to"`
	BlockNum                     int64       `json:"block_num"`
	BlockTime                    int64       `json:"block_time"`
	SwapName                     string      `json:"swap_name"`
	CurrentTokenInPoolAmount     float64     `json:"current_token_in_pool_amount"`
	CurrentBaseTokenInPoolAmount float64     `json:"current_base_token_in_pool_amount"`
	Fdv                          float64     `json:"fdv"`
	Mcap                         float64     `json:"mcap"`
	Clamp                        bool        `json:"-"`
	PairInfo                     PairMessage `json:"pair_info"`
}

type PairMessage struct {
	ChainID                string  `json:"chain_id"`
	Addr                   string  `json:"addr"`
	BaseTokenAddr          string  `json:"base_token_addr"`
	TokenAddr              string  `json:"token_addr"`
	BaseTokenSymbol        string  `json:"base_token_symbol"`
	TokenSymbol            string  `json:"token_symbol"`
	BaseTokenDecimal       uint8   `json:"base_token_decimal"`
	TokenDecimal           uint8   `json:"token_decimal"`
	BaseTokenIsNativeToken bool    `json:"base_token_is_native_token"`
	BaseTokenIsToken0      bool    `json:"base_token_is_token_0"`
	TokenTotalSupply       float64 `json:"token_total_supply"`
	InitTokenAmount        float64 `json:"init_token_amount"`
	InitBaseTokenAmount    float64 `json:"init_base_token_amount"`
	Name                   string  `json:"name"`
	BlockNum               int64   `json:"block_num"`
	BlockTime              int64   `json:"block_time"`
	CurrentBaseTokenAmount float64 `json:"current_base_token_amount"`
	CurrentTokenAmount     float64 `json:"current_token_amount"`
}
