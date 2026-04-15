package entity

import "encoding/json"

type RedisTokenPriceLimitOrderInfo struct {
	OrderId   int64
	BasePrice string
}



func (r *RedisTokenPriceLimitOrderInfo) Serialize() (string, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
