package entity

type OrderMessage struct {
	TokenCA      string
	CurrentPrice string
	SwapType     int64
	ChainId      int
}
