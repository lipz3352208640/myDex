package solana

import (
	"context"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
)

const accountSize = 165

type RentService interface {
	UpdateRentFee() (uint64, error)
	CheckRentFee()
}

func (tx *TxManager) CheckRentFee() {
	//默认lamport 2039280,设置兜底机制，防止程序刚启动没访问rpc
	tx.rentFee = 2039280
	rentFee, err := tx.UpdateRentFee()
	if err != nil {
		tx.Errorf("CheckRentFee:UpdateRentFee GetMinimumBalanceForRentExemption err:%v ", err)
	} else {
		tx.rentFee = rentFee
	}

	updateRentTimer := time.NewTicker(1 * time.Minute)
	for {
		select {
		case <-updateRentTimer.C:
			rentFee, err := tx.UpdateRentFee()
			if err != nil {
				tx.Errorf("CheckRentFee:UpdateRentFee GetMinimumBalanceForRentExemption err:%v ", err)
			} else {
				tx.rentFee = rentFee
			}
		}
	}
}

func (tx *TxManager) UpdateRentFee() (uint64, error) {
	//创建带超时的上下文，防止卡死。每次请求单独创建一个，互不影响
	const retries = 3
	const timeout = 10 * time.Second
	var lastErr error

	for attempt := 0; attempt < retries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		lamport, err := tx.Client.GetMinimumBalanceForRentExemption(ctx, accountSize, rpc.CommitmentProcessed)
		cancel()
		if err == nil {
			return lamport, nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}

	return 0, lastErr
}
