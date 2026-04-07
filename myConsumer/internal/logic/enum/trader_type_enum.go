package enum

type TradeType uint8

const (
	TradeTypeUnknown TradeType = iota
	TradeTypeBuy
	TradeTypeSell
)

var tradeTypeName = map[TradeType]string{
	TradeTypeUnknown: "unknown",
	TradeTypeBuy:     "buy",
	TradeTypeSell:    "sell",
}

func (t TradeType) String() string {
	if s, ok := tradeTypeName[t]; ok {
		return s
	}
	return "unknown"
}

func TraderTypeFromCode(code int) TradeType {
	switch code {
	case 1:
		return TradeTypeBuy
	case 2:
		return TradeTypeSell
	default:
		return TradeTypeUnknown
	}
}
