package entity

type TokenAccount struct {
	Owner                   string //账户所属者
	TokenAccountAddress     string //账户地址
	TokenMintAccountAddress string //tokin mint 账户地址
	TokenDecimal            uint8    //代币的小数位数
	PreValue                int64  //账户余额变动之前余额
	PostValue               int64  //账户余额变动之后的金额
	Closed                  bool   //账户是否被关闭
	Init                    bool   //账户是否被初始化
	PreValueUIString        string //格式化后的PreValue
	PostValueUIString       string //格式化后的PostValue

}