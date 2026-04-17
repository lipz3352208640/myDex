package ticker

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"myDex/market/market"
	"myDex/model/solmodel"
	"myDex/pkg/constant"
	"myDex/trade/internal/enum"
	"myDex/trade/internal/logic"
	"myDex/trade/internal/svc"
	"myDex/trade/trade"
	"net/http"
	"strings"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/compute_budget"
	"github.com/blocto/solana-go-sdk/program/system"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/near/borsh-go"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
)

type DoubleOutTicker struct {
	// Define fields for the ticker
	ctx    context.Context
	cancle func(err error)
	logx.Logger
	svcCtx *svc.ServiceContext
	c      *client.Client
}

func NewDoubleOutTicker(svcCtx *svc.ServiceContext) *DoubleOutTicker {
	solClient := client.New(rpc.WithEndpoint(svcCtx.Config.Helius.NodeUrl[0]), rpc.WithHTTPClient(&http.Client{
		Timeout: 10 * time.Second,
	}))
	ctx, cancle := context.WithCancelCause(context.Background())
	return &DoubleOutTicker{
		ctx:    ctx,
		cancle: cancle,
		Logger: logx.WithContext(ctx),
		svcCtx: svcCtx,
		c:      solClient,
	}
}

type FeeInfo struct {
	FinalAmount    float64
	FinalPriceBase float64
	FinalValueBase float64
	FinalBasePrice float64
	GasFee         float64
	PriorityFee    float64
	DexFee         float64
	ServerFee      float64
	JitoFee        float64
}

type ComputeInstruction struct {
	Instruction compute_budget.Instruction
	Units       uint32
}

type SystemInstruction struct {
	Instruction system.Instruction
	Lamports    uint64
}

var (
	jitoTipAddresses = []common.PublicKey{
		common.PublicKeyFromString("96gYZGLnJYVFmbjzopPSU6QiEV5fGqZNyN9nmNhvrZU5"),
		common.PublicKeyFromString("HFqU5x63VTqvQss8hp11i4wVV8bD44PvwucfZ2bU7gRe"),
		common.PublicKeyFromString("Cw8CFyM9FkoMi7K7Crf6HNQqf4uEMzpKw6QNghXLvLkY"),
		common.PublicKeyFromString("ADaUMid9yfUytqMBgopwjb2DTLSokTSzL1zt6iGPaS49"),
		common.PublicKeyFromString("DfXygSm4jCyNCybVYYK6DwvWqjKee8pbDmJGcLWNDXjh"),
		common.PublicKeyFromString("ADuUkR4vqLUMWXxW9gh6D6L8pMSawimctcNZ5pGwDcEt"),
		common.PublicKeyFromString("DttWaMuVvTiduZRnguLF7jNxTgiMBZ1hyAumKUiL2KRL"),
		common.PublicKeyFromString("3AVi9Tg9Uo68tJfuvoKvqKNWKkC5wPdSSdeBnizKZ6jT"),
	}
	pumpFunFeeRecipient = common.PublicKeyFromString("CebN5WGQ4jvEPvsVU4EoHEpgzq1VV7AbicfhtW4xC9iM")
)

func (t *DoubleOutTicker) Start() {
	// Start the ticker
	t.Info("ticker started")
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			t.Info("ticker context done")
			return
		case <-ticker.C:
			t.Info("ticker ticked")
			t.ExecuteDoubleOut()
		}
	}
}
func (t *DoubleOutTicker) ExecuteDoubleOut() {
	// Execute the double out logic
	t.Info("executing double out logic")

	var (
		offset int = 0
	)
	for {
		//step 1: update order status base on chain tx
		tradeOrderData, err := t.svcCtx.TradeOrderModel.FindOnChainOrderByChainId(t.ctx, int64(constant.SolChainIdInt), 10, offset)
		if err != nil {
			t.Error("failed to find on-chain orders", err)
			offset += 10
			continue
		}

		if len(tradeOrderData) == 0 {
			t.Info("no on-chain orders found")
			break
		}

		//step 2: get on-chain tx status and update order status
		queryItem := lo.Map(tradeOrderData, func(tradeOrder *solmodel.TradeOrder, index int) *solmodel.Trade {
			return &solmodel.Trade{
				TxHash: tradeOrder.TxHash,
				// Query with a slightly earlier lower bound to avoid missing
				// trade rows whose created_at is earlier than the order row.
				CreatedAt: tradeOrder.CreatedAt,
			}
		})
		tradeModel := solmodel.NewTradeModel(t.svcCtx.DB)
		tradeData, err := tradeModel.FindByTxHashAndCreateTimes(t.ctx, queryItem)
		if err != nil {
			t.Error("failed to query trades by tx hash and create time", err)
			offset += 10
			continue
		}

		if len(tradeData) == 0 {
			t.Info("no matched trades found")
			offset += 10
			continue
		}

		tradeMap := lo.SliceToMap(tradeData, func(item *solmodel.Trade) (string, *solmodel.Trade) {
			return item.TxHash, item
		})

		for _, order := range tradeOrderData {
			dbTrade, ok := tradeMap[order.TxHash]
			if !ok || dbTrade == nil {
				continue
			}

			//step 3: get fee info by decode transaction instruction
			feeInfo, err := t.GetFeeInfoByDecodeInstruction(order, dbTrade)
			if err != nil {
				t.Errorf("failed to get fee info, orderId=%d txHash=%s err=%v", order.Id, order.TxHash, err)
				continue
			}

			order.GasFee = feeInfo.GasFee
			order.PriorityFee = feeInfo.PriorityFee
			order.DexFee = feeInfo.DexFee
			order.ServerFee = feeInfo.ServerFee
			order.JitoFee = feeInfo.JitoFee

			order.Status = int64(enum.OrderStatus_Suc)
			order.FinalAmount = feeInfo.FinalAmount
			order.FinalPriceBase = feeInfo.FinalPriceBase
			order.FinalValueBase = feeInfo.FinalValueBase
			order.FinalBasePrice = feeInfo.FinalBasePrice

			ctx, cancle := context.WithTimeout(context.Background(), 5*time.Second)
			pair, err := t.svcCtx.Marketclient.FindPairInfoByPairAddress(ctx, &market.PairInfoReq{
				PairAddr: dbTrade.PairAddr,
				ChainId:  int64(constant.SolChainIdInt),
			})
			cancle()
			ctx, cancle = context.WithTimeout(context.Background(), 5*time.Second)
			if err != nil {
				pair, err = t.svcCtx.Marketclient.FindMaxSupplyPairInfoByTokenAddrAndChainID(ctx, &market.PairInfoRequest{
					TokenAddr: order.TokenCa,
					ChainId:   int64(constant.SolChainIdInt),
				})
				cancle()
				if err != nil {
					t.Errorf("failed to get pair info, orderId=%d txHash=%s pairAddr=%s err=%v", order.Id, order.TxHash, dbTrade.PairAddr, err)
					continue
				}
			} else {
				cancle()
			}
			order.OrderCap = decimal.NewFromFloat32(float32(pair.Fdv))

			t.Infof("matched trade order, orderId=%d txHash=%s tradeId=%d gasFee=%f priorityFee=%f dexFee=%f serverFee=%f jitoFee=%f",
				order.Id, order.TxHash, dbTrade.Id, feeInfo.GasFee, feeInfo.PriorityFee, feeInfo.DexFee, feeInfo.ServerFee, feeInfo.JitoFee)

			ctx, cancle = context.WithTimeout(context.Background(), 5*time.Second)
			if err = t.svcCtx.TradeOrderModel.Update(ctx, order); err == nil {
				if isSuccess := t.processDoubleOut(order); isSuccess {
					fmt.Println("double out success")
				}
			}
			cancle()

		}
		offset += 10
	}

}

func (t *DoubleOutTicker) processDoubleOut(tradeOrder *solmodel.TradeOrder) bool {

	t.Infof("processing double out, orderId=%d txHash=%s", tradeOrder.Id, tradeOrder.TxHash)
	if tradeOrder.Status != int64(enum.OrderStatus_Suc) || tradeOrder.DoubleOut != 1 || tradeOrder.SwapType != int64(trade.SwapType_Buy) {
		return false
	}

	// step 1: calculate double out token usd price
	tokenPriceUSD := decimal.NewFromFloat32(float32(tradeOrder.FinalPriceBase)).
		Mul(decimal.NewFromFloat32(float32(tradeOrder.FinalBasePrice))).Mul(decimal.NewFromInt(2))

	// step 2: calculate double out token amount
	newTokenAmount := decimal.NewFromFloat32(float32(tradeOrder.FinalAmount)).Div(decimal.NewFromInt(2))

	// step 3: calculate base price。 1sol -> 5 usd  1token -> 10 usd
	newPrice := tokenPriceUSD.Div(decimal.NewFromFloat32(float32(tradeOrder.FinalBasePrice)))

	newOrderCap := decimal.NewFromFloat32(float32(tradeOrder.FinalCap)).Mul(decimal.NewFromInt(2))

	newOrder := &solmodel.TradeOrder{
		ChainId:        tradeOrder.ChainId,
		Uid:            tradeOrder.Uid,
		TradeType:      int64(enum.TradeType_Limit),
		WalletIndex:    tradeOrder.WalletIndex,
		WalletAddress:  tradeOrder.WalletAddress,
		GasType:        tradeOrder.GasType,
		IsAutoSlippage: tradeOrder.IsAutoSlippage,
		Slippage:       tradeOrder.Slippage,
		IsAntiMev:      tradeOrder.IsAntiMev,
		TokenCa:        tradeOrder.TokenCa,
		SwapType:       int64(trade.SwapType_Sell),
		OrderCap:       newOrderCap,
		OrderAmount:    newTokenAmount,
		OrderValueBase: newPrice.Mul(newTokenAmount),
		OrderPriceBase: newPrice,
		OrderBasePrice: decimal.NewFromFloat32(float32(tradeOrder.FinalBasePrice)),
		Status:         int64(enum.OrderStatus_Wait),
		DoubleOut:      1, // 记录下这个单是翻倍出本触发的
	}

	if err := logic.NewCreateLimitMarketOrderLogic(t.ctx, t.svcCtx).CreateTradeOrder(newOrder); err != nil {
		t.Errorf("failed to create double out limit order, orderId=%d txHash=%s err=%v", tradeOrder.Id, tradeOrder.TxHash, err)
		return false
	}
	return true

}

func (t *DoubleOutTicker) GetFeeInfoByDecodeInstruction(order *solmodel.TradeOrder, tradeData *solmodel.Trade) (*FeeInfo, error) {
	if order == nil || tradeData == nil {
		return nil, fmt.Errorf("order or tradeData is nil")
	}

	var (
		gasFee           float64
		priorityFee      float64
		jitoFee          float64
		serverFee        float64
		dexFee           float64
		computeUnitLimit uint32
		computeUnitPrice uint32
	)

	transaction, err := t.c.GetTransactionWithConfig(t.ctx, tradeData.TxHash, client.GetTransactionConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return nil, err
	}
	if transaction == nil || transaction.Meta == nil {
		return nil, fmt.Errorf("transaction meta is nil, txHash=%s", tradeData.TxHash)
	}

	accountKeys := transaction.AccountKeys
	for _, item := range transaction.Transaction.Message.Instructions {
		if len(accountKeys) <= item.ProgramIDIndex {
			continue
		}
		//step 1: decode compute budget instruction to get compute unit limit and compute unit price
		if accountKeys[item.ProgramIDIndex] == common.ComputeBudgetProgramID {
			computeInstruction := &ComputeInstruction{}
			if err = borsh.Deserialize(computeInstruction, item.Data); err != nil {
				continue
			}
			if computeInstruction.Instruction == compute_budget.InstructionSetComputeUnitLimit {
				computeUnitLimit = computeInstruction.Units
				continue
			}
			if computeInstruction.Instruction == compute_budget.InstructionSetComputeUnitPrice {
				computeUnitPrice = computeInstruction.Units
			}
			continue
		}
		//step 2: decode system instruction to get transfer info, including server fee and jito fee
		if accountKeys[item.ProgramIDIndex] != common.SystemProgramID {
			continue
		}

		systemInstruction := &SystemInstruction{}
		if err = borsh.Deserialize(systemInstruction, item.Data); err != nil {
			continue
		}
		if systemInstruction.Instruction != system.InstructionTransfer || len(item.Accounts) < 2 || len(accountKeys) <= item.Accounts[1] {
			continue
		}

		to := accountKeys[item.Accounts[1]]
		amount := decimal.New(int64(systemInstruction.Lamports), -int32(constant.SolDecimal)).InexactFloat64()
		if to.String() == constant.FeeReceiver {
			serverFee = amount
			continue
		}
		if lo.Contains(jitoTipAddresses, to) {
			jitoFee = amount
		}
	}

	//step 3: decode inner instruction to get dex fee
	for _, inner := range transaction.Meta.InnerInstructions {
		for _, item := range inner.Instructions {
			if len(accountKeys) <= item.ProgramIDIndex || accountKeys[item.ProgramIDIndex] != common.SystemProgramID {
				continue
			}

			systemInstruction := &SystemInstruction{}
			if err = borsh.Deserialize(systemInstruction, item.Data); err != nil {
				continue
			}
			if systemInstruction.Instruction != system.InstructionTransfer || len(item.Accounts) < 2 || len(accountKeys) <= item.Accounts[1] {
				continue
			}

			if accountKeys[item.Accounts[1]] == pumpFunFeeRecipient {
				dexFee = decimal.New(int64(systemInstruction.Lamports), -int32(constant.SolDecimal)).InexactFloat64()
			}
		}
	}

	gasFee = decimal.New(int64(transaction.Meta.Fee), -int32(constant.SolDecimal)).InexactFloat64()
	if computeUnitLimit > 0 && computeUnitPrice > 0 {
		priorityFee = decimal.NewFromBigInt(
			new(big.Int).Mul(big.NewInt(int64(computeUnitLimit)), big.NewInt(int64(computeUnitPrice))),
			-int32(constant.SolDecimal+6),
		).InexactFloat64()
	}

	baseTokenAmount := tradeData.BaseTokenAmount
	baseTokenPriceUsd := tradeData.BaseTokenPriceUsd
	tokenAmount := tradeData.TokenAmount
	tokenPriceUsd := tradeData.TokenPriceUsd
	finalAmount := tokenAmount

	finalPriceBase := decimal.NewFromFloat32(float32(tokenPriceUsd)).
		Div(decimal.NewFromFloat32(float32(baseTokenPriceUsd)))
	finalValueBase := decimal.NewFromFloat32(float32(tokenAmount)).Mul(finalPriceBase)
	fmt.Println("tradeData.TradeType:", tradeData.TradeType)
	fmt.Println("tradeData.SwapType_Buy:", trade.SwapType_Buy.String())

	if strings.ToUpper(tradeData.TradeType) == strings.ToUpper(trade.SwapType_Buy.String()) {
		fmt.Printf(
			"this is a buy order, base token amount: %f, base token price usd: %f, token amount: %f, token price usd: %f\n",
			baseTokenAmount,
			baseTokenPriceUsd,
			tokenAmount,
			tokenPriceUsd,
		)
		finalAmount = baseTokenAmount
		finalValueBase = decimal.NewFromFloat32(float32(baseTokenAmount))
	}

	return &FeeInfo{
		FinalAmount:    finalAmount,
		FinalPriceBase: finalPriceBase.InexactFloat64(),
		FinalValueBase: finalValueBase.InexactFloat64(),
		FinalBasePrice: tradeData.BaseTokenPriceUsd,
		GasFee:         gasFee,
		PriorityFee:    priorityFee,
		DexFee:         dexFee,
		ServerFee:      serverFee,
		JitoFee:        jitoFee,
	}, nil
}

func (t *DoubleOutTicker) Stop() {
	// Stop the ticker
	t.Info("ticker stopped")
	t.cancle(errors.New("ticker stopped"))
}
