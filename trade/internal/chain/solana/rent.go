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
	context, cancle := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancle()
	lamport, err := tx.Client.GetMinimumBalanceForRentExemption(context, accountSize, rpc.CommitmentFinalized)
	if err != nil {
		return 0, err
	}
	return lamport, nil
}
