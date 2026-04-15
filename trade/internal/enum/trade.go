package enum

type TradeType uint8

const (
	TradeType_Market        TradeType = iota + 1 //市价单
	TradeType_Limit                              // 限价单
	TradeType_OneClick                           // 一键买卖
	TradeType_TokenCapLimit                      // 按照市值限价单
	TradeType_TrailingStop                       // 移动止损类型
)

type OrderStatus uint8

const (
	OrderStatus_Wait         OrderStatus = iota + 1 // 等待中 订单已经创建，但还没开始真正处理
	OrderStatus_Proc                                // 处理中  订单已经进入执行流程了
	OrderStatus_OnChain                             // 已上链 通常表示交易已经发出去了，链上已经有 tx hash
	OrderStatus_Fail                                // 失败
	OrderStatus_Suc                                 // 成功
	OrderStatus_cancel                              // 订单被主动取消了 这个多见于限价单、追踪止损单、定时单这种“还没触发前可以撤销”的订单
	OrderStatus_timeout_fail                        // 超时失败
)
