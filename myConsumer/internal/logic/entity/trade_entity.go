package entity

// 拥有交易对的交易对象
type TradeWithPair struct {
	Slot                         int64   `json:"slot"`                    // slot
	ChainId                      string  `json:"chain_id" tag:"true"`     // 使用的哪个链，solana链/以太坊链
	ChainIdInt                   int     `json:"chain_id_int" tag:"true"` // 链id
	PairAddr                     string  `json:"pair_addr" tag:"true"`    // 池子/价格曲线状态地址
	TxHash                       string  `json:"tx_hash" tag:"true"`      // 交易hash
	HashId                       string  `json:"hash_id"`
	Maker                        string  `json:"maker"`                             // 钱包地址
	Type                         string  `json:"type"`                              // 交易类型: sell/buy/add_position/remove_position
	BaseTokenAmount              float64 `json:"base_token_amount"`                 // 基础代币变动数量
	TokenAmount                  float64 `json:"token_amount"`                      // 另一个代币变动数量
	BaseTokenPriceUSD            float64 `json:"base_token_price_usd"`              // 基础代币美元价格
	TotalUSD                     float64 `json:"total_usd"`                         // 美元计价总数
	TokenPriceUSD                float64 `json:"token_price_usd"`                   // 另一个代币美元价格
	To                           string  `json:"to"`                                // 代币接收方地址
	BlockNum                     int64   `json:"block_num"`                         // 区块高度
	BlockTime                    int64   `json:"block_time"`                        // 区块出块事件
	TransactionIndex             int     `json:"transaction_index"`                 // 交易在交易列表中的索引
	LogIndex                     int     `json:"log_index"`                         // 交易日志在日志列表中的索引
	SwapName                     string  `json:"swap_name"`                         // 交易池版本
	CurrentTokenInPoolAmount     float64 `json:"current_token_in_pool_amount"`      // 池子当前货币数量
	CurrentBaseTokenInPoolAmount float64 `json:"current_base_token_in_pool_amount"` // 池子当前基础代币数量

	// KlineUpDown5m  float64 `json:"kline_up_down_5m"`  // 5-minute price change, used for pushing to websocket
	// KlineUpDown1h  float64 `json:"kline_up_down_1h"`  // 1-hour price change, used for pushing to websocket
	// KlineUpDown4h  float64 `json:"kline_up_down_4h"`  // 4-hour price change, used for pushing to websocket
	// KlineUpDown6h  float64 `json:"kline_up_down_6h"`  // 6-hour price change, used for pushing to websocket
	// KlineUpDown24h float64 `json:"kline_up_down_24h"` // 24-hour price change, used for pushing to websocket
	Fdv  float64 `json:"fdv"`  // Market cap, used for pushing to websocket
	Mcap float64 `json:"mcap"` // Circulating market cap

	// TokenAmountInt     int64 `json:"token_amount_int"` // Not divided by decimal
	// BaseTokenAmountInt int64 `json:"base_token_amount_int"`
	// Clamp              bool  `json:"clamp"` // true: clamped or in a clamp
	// Clipper            bool  `json:"-"`     // true: clamp

	// pump
	// PumpPoint                    float64   `json:"pump_point"`    // Pump score
	// PumpLaunched                 bool      `json:"pump_launched"` // Pump launched
	// PumpMarketCap                float64   `json:"pump_market_cap"`
	// PumpOwner                    string    `json:"pump_owner"`
	// PumpSwapPairAddr             string    `json:"pump_swap_pair_addr"`
	// PumpVirtualBaseTokenReserves float64   `json:"pump_virtual_base_token_reserves,omitempty"`
	// PumpVirtualTokenReserves     float64   `json:"pump_virtual_token_reserves,omitempty"`
	// PumpStatus                   int       `json:"pump_status"`
	// PumpPairAddr                 string    `json:"pump_pair_addr"`
	// CreateTime                   time.Time `json:"create_time"`

	// sol
	// BaseTokenAccountAddress string `json:"-"`
	// TokenAccountAddress     string `json:"-"`

	PairInfo Pair `json:"pair_info"`
}

// 池子交易对对象
type Pair struct {
	ChainId string `json:"chain_id"` //链id
	Addr    string `json:"addr"`     //池子或者价格曲线状态账户地址

	BaseTokenAddr          string `json:"base_token_addr"`            //基础代币地址
	TokenAddr              string `json:"token_addr"`                 //代币地址
	BaseTokenSymbol        string `json:"base_token_symbol"`          //基础代币
	TokenSymbol            string `json:"token_symbol"`               //代币标志
	BaseTokenDecimal       uint8  `json:"base_token_decimal"`         //基础代币精度
	TokenDecimal           uint8  `json:"token_decimal"`              //代币精度
	BaseTokenIsNativeToken bool   `json:"base_token_is_native_token"` //基础代币是否是原生token
	BaseTokenIsToken0      bool   `json:"base_token_is_token_0"`      //基础代币是否对应pair中的token0

	TokenTotalSupply    float64 `json:"token_total_supply"`     // 代币总供应量
	InitTokenAmount     float64 `json:"init_token_amount"`      // 池子初始化代币数量
	InitBaseTokenAmount float64 `json:"init_base_token_amount"` // 池子初始化基础代币数量

	Name string `json:"name"` //交易名。属于哪个dex平台

	BlockNum  int64 `json:"block_num"`  // 池子创建Slot
	BlockTime int64 `json:"block_time"` // 池子创建时间

	CurrentBaseTokenAmount float64 `gorm:"column:current_base_token_amount"` // 当前base流动性数量
	CurrentTokenAmount     float64 `gorm:"column:current_token_amount"`      // 当前token流动性数量
}
