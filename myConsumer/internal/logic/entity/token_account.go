package entity

import (
	"github.com/blocto/solana-go-sdk/program/metaplex/token_metadata"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/shopspring/decimal"
)

type TokenAccount struct {
	Owner                   string //账户所属者
	TokenAccountAddress     string //账户地址
	TokenMintAccountAddress string //tokin mint 账户地址
	TokenDecimal            uint8  //代币的小数位数
	PreValue                int64  //账户余额变动之前余额
	PostValue               int64  //账户余额变动之后的金额
	Closed                  bool   //账户是否被关闭
	Init                    bool   //账户是否被初始化
	PreValueUIString        string //格式化后的PreValue
	PostValueUIString       string //格式化后的PostValue

}

type TokenUriData struct {
	Twitter     string `json:"twitter"`
	Website     string `json:"website"`
	Telegram    string `json:"telegram"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	Symbol      string `json:"symbol"`
	Description string `json:"description"`
}

type TokenInfo struct {
	ProgramId     string
	MintCount     token.MintAccount
	MetaData      token_metadata.Metadata
	Uri           TokenUriData
	TotalSupply   decimal.Decimal
	IsCanAddToken uint8
	IsDropFreeze  uint8
	HoldersCount  int64
	Name          string
	Symbol        string
}
type MintResponse struct {
	Parsed  Parsed `json:"parsed"`
	Program string `json:"program"`
	Space   int    `json:"space"`
}

type Parsed struct {
	Info Info   `json:"info"`
	Type string `json:"type"`
}

type Info struct {
	Decimals        int         `json:"decimals"`
	Extensions      []Extension `json:"extensions"`
	FreezeAuthority *string     `json:"freezeAuthority"` // 使用指针以处理可能为 null 的情况
	IsInitialized   bool        `json:"isInitialized"`
	MintAuthority   string      `json:"mintAuthority"`
	Supply          string      `json:"supply"`
}

type Extension struct {
	Extension string `json:"extension"`
	State     State  `json:"state"`
}

type State struct {
	Authority       *string `json:"authority"` // 使用指针以处理可能为 null 的情况
	MetadataAddress string  `json:"metadataAddress,omitempty"`
	// AdditionalMetadata []string `json:"additionalMetadata"`
	Mint            string `json:"mint"`
	Name            string `json:"name"`
	Symbol          string `json:"symbol"`
	UpdateAuthority string `json:"updateAuthority"`
	Uri             string `json:"uri"`
}
