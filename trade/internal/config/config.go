package config

import "github.com/zeromicro/go-zero/zrpc"

type Config struct {
	zrpc.RpcServerConf
	MarketService zrpc.RpcClientConf `json:"market_service"`
	Mysql         MysqlConfig        `json:"mysql"`
	Helius        Entity             `json:"Helius,optional"`
	Jupiter       JupiterConfig      `json:"jupiter,optional"`
	Arbitrage     ArbitrageConfig    `json:"arbitrage,optional"`
	SimulateOnly  bool               `json:"simulate_only"`
}

type MysqlConfig struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Dbname   string `json:"dbname"`
}

type Entity struct {
	NodeUrl []string `json:"NodeUrl"`
	JitoUrl string   `json:"JitoUrl,optional" json:",env=SOL_JITOURL"`
	WSUrl   string   `json:"WSUrl,optional" json:",env=SOL_WSURL"`
}

type JupiterConfig struct {
	QuoteURL                string `json:"quote_url,optional"`
	SwapInstructionsURL     string `json:"swap_instructions_url,optional"`
	JitoBundleURL           string `json:"jito_bundle_url,optional"`
	ProfitThresholdLamports int64  `json:"profit_threshold_lamports,optional"`
	TipBps                  uint64 `json:"tip_bps,optional"`
}

type ArbitrageConfig struct {
	Enabled          bool   `json:"enabled,optional"`
	IntervalMs       int64  `json:"interval_ms,optional"`
	StartMint        string `json:"start_mint,optional"`
	MidMint          string `json:"mid_mint,optional"`
	AmountLamports   uint64 `json:"amount_lamports,optional"`
	SlippageBps      uint32 `json:"slippage_bps,optional"`
	MaxAccounts      uint32 `json:"max_accounts,optional"`
	JitoTipRecipient string `json:"jito_tip_recipient,optional"`
}
