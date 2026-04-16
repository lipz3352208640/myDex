package block

import (
	"context"
	"errors"
	"fmt"
	"myDex/model/solmodel"
	"myDex/myConsumer/internal/logic/entity"
	"myDex/myConsumer/internal/logic/enum"
	"myDex/myConsumer/internal/svc"
	"myDex/pkg/constant"
	"myDex/trade/trade"
	"myDex/trade/tradeclient"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
)

type TradeService interface {
	SaveTrades(trades []*entity.TradeWithPair) error
	UpdateTrade() error
	Stop()
}

type TradeServiceImpl struct {
	ctx    context.Context
	cancel func(error error)
	logx.Logger
	sc           *svc.ServiceContext
	tokenService TokenService
}

func (t *TradeServiceImpl) SaveTrades(trades []*entity.TradeWithPair) error {
	validTrades := lo.Filter(trades, func(trade *entity.TradeWithPair, _ int) bool {
		if trade == nil {
			return false
		}

		if trade.TxHash == "" {
			return false
		}
		if trade.BaseTokenPriceUSD == 0 {
			return false
		}
		if "" == trade.Type || (trade.Type != enum.TradeTypeBuy.String() && trade.Type != enum.TradeTypeSell.String()) {
			return false
		}
		return true
	})

	for i, tr := range trades {
		if tr == nil {
			fmt.Printf("[%d] trade=nil\n", i)
			continue
		}
		fmt.Printf("[%d] txHash=%q price=%v type=%q buy=%q sell=%q\n",
			i,
			tr.TxHash,
			tr.BaseTokenPriceUSD,
			tr.Type,
			enum.TradeTypeBuy.String(),
			enum.TradeTypeSell.String(),
		)
	}

	tradeMap := lo.GroupBy(validTrades, func(trade *entity.TradeWithPair) string {
		return trade.PairAddr
	})

	gp := threading.NewRoutineGroup()

	fmt.Println("input trades len:", len(trades))
	fmt.Println("validTrades len:", len(validTrades))
	fmt.Println("tradeMap len:", len(tradeMap))

	for key, trades := range tradeMap {

		gp.RunSafe(func(pair string, trades []*entity.TradeWithPair) func() {
			return func() {

				txHashList := lo.Map(trades, func(trade *entity.TradeWithPair, _ int) string {
					return trade.TxHash
				})
				t.Info("current trade transcation is %v", txHashList)
				t.BatchSaveByTrade(trades)

			}
		}(key, trades))
	}
	gp.Wait()
	return nil
}

func (t *TradeServiceImpl) BatchSaveByTrade(trades []*entity.TradeWithPair) {
	select {
	case <-t.ctx.Done():
		return
	default:
	}

	if len(trades) == 0 {
		t.Info("trade is empty slice")
		return
	}

	for _, trade := range trades {
		tokenDb, err := t.tokenService.SaveToken(trade)
		if err != nil {
			t.Errorf("BatchSaveByTrade:SaveToken err is %w", err)
		}
		if tokenDb.TotalSupply == 0 {
			t.Errorf("trade PairInfo token totalSupply is 0, tokenDb: %#v ", tokenDb)
		} else {
			trade.PairInfo.TokenTotalSupply = tokenDb.TotalSupply
		}

		_, err = t.SavePair(t.ctx, trade, tokenDb)
		if err != nil {
			t.Errorf("BatchSaveByTrade:SavePair err is %w", err)
		}

		err = t.SaveTrade(t.ctx, trade)
		if err != nil {
			t.Errorf("BatchSaveByTrade:SaveTrade err is %w", err)
		}

		t.sendTokenPrice2Trade(t.ctx, trade)

	}
	fmt.Println("save BatchSaveByTrade successful")

}

func (t *TradeServiceImpl) SaveTrade(ctx context.Context, trade *entity.TradeWithPair) (err error) {
	if trade == nil {
		return fmt.Errorf("SaveTrade: trade is nil")
	}
	if t.sc == nil || t.sc.TradeModel == nil {
		return fmt.Errorf("SaveTrade: trade model is nil")
	}

	tradeAtDB := &solmodel.Trade{
		ChainId:           int64(trade.ChainIdInt),
		PairAddr:          trade.PairAddr,
		TxHash:            trade.TxHash,
		HashId:            trade.HashId,
		Maker:             trade.Maker,
		TradeType:         trade.Type,
		BaseTokenAmount:   trade.BaseTokenAmount,
		TokenAmount:       trade.TokenAmount,
		BaseTokenPriceUsd: trade.BaseTokenPriceUSD,
		TokenPriceUsd:     trade.TokenPriceUSD,
		TotalUsd:          trade.TotalUSD,
		To:                trade.To,
		BlockNum:          trade.BlockNum,
		BlockTime:         time.Unix(trade.BlockTime, 0),
		SwapName:          trade.SwapName,
	}
	data, err := t.sc.TradeModel.FindOneByHashId(ctx, trade.HashId)
	if err != nil || data == nil {
		t.Errorf("SaveTrade insert failed: txHash=%s hashId=%s err=%v", trade.TxHash, trade.HashId, err)
		if errors.Is(err, solmodel.ErrNotFound) {
			err = t.sc.TradeModel.Insert(ctx, tradeAtDB)
			if err != nil {
				return fmt.Errorf("SaveTrade:Insert err is %w", err)
			}
		}
	}
	return nil
}

func (t *TradeServiceImpl) sendTokenPrice2Trade(ctx context.Context, tradeWithPair *entity.TradeWithPair) {

	if tradeWithPair.Type != enum.TradeTypeBuy.String() && tradeWithPair.Type != enum.TradeTypeSell.String() {
		t.Infof("sendTokenPrice2Trade tradeWithPair type in not buy or sell,tx hash: %v", tradeWithPair.TxHash)
		return
	}

	if tradeWithPair.PairInfo.TokenAddr == "HmRbt3nXAKHzFL9agaDPrVFWtsCzKbKth4YPgHCYpump" {
		fmt.Println("tradewithpair pairInfo tokenAddr is:", tradeWithPair.PairInfo.TokenAddr)
	}

	token2BasePrice := decimal.NewFromFloat(tradeWithPair.TokenPriceUSD).Div(decimal.NewFromFloat(tradeWithPair.BaseTokenPriceUSD)).String()

	_, err := t.sc.TradeService.ProcTokenPrice(ctx, &tradeclient.ProcTokenPriceRequest{
		TokenCa:  tradeWithPair.PairInfo.TokenAddr,
		Price:    token2BasePrice,
		SwapType: trade.SwapType_Buy,
		ChainId:  constant.SolChainIdInt,
	})
	if err != nil {
		logx.Errorf("sendTokenPrice2Trade failed: pair=%v, err=%v", tradeWithPair.PairAddr, err)
		return
	}
	fmt.Println("successfully transfer token price info to trade service")
}
func (t *TradeServiceImpl) SavePair(ctx context.Context, trade *entity.TradeWithPair, tokenDb *solmodel.Token) (pairAtDB *solmodel.Pair, err error) {

	//chainID 转换为int
	chainInt, _ := strconv.ParseInt(trade.ChainId, 10, 64)

	//代币总发行量
	var tokenTotalSupply float64
	//代币符号
	var tokenSymbol = trade.PairInfo.TokenSymbol
	if tokenDb != nil {
		tokenTotalSupply = tokenDb.TotalSupply
		tokenSymbol = tokenDb.Symbol
	}

	pairAtDB, err = t.sc.PairModel.FindOneByChainIdAddress(ctx, chainInt, trade.PairAddr)
	//默认流动性计算：当前池子a代币数量*a的价格 + 当前池子b代币数量*b的价格
	liq := trade.CurrentBaseTokenInPoolAmount*trade.BaseTokenPriceUSD + trade.CurrentTokenInPoolAmount*trade.TokenPriceUSD
	if trade.SwapName == constant.PumpFunName {
		//如果是pump 池子双边价值相等
		liq = trade.CurrentBaseTokenInPoolAmount * trade.BaseTokenPriceUSD * 2
		t.Infof("SavePair: calculated liquidity (PumpFun formula) for pair %v: liq=%v, CurrentBaseTokenInPoolAmount=%v, BaseTokenPriceUSD=%v", trade.PairAddr, liq, trade.CurrentBaseTokenInPoolAmount, trade.BaseTokenPriceUSD)
	}

	switch {
	case errors.Is(err, solmodel.ErrNotFound):
		var baseTokenIsNativeToken, baseTokenIsToken0 int64
		//如果是原生代币  状态置为1，默认为0
		if trade.PairInfo.BaseTokenIsNativeToken {
			baseTokenIsNativeToken = 1
		}

		//如果是pair交易对中的token0  状态置为1，默认为0
		if trade.PairInfo.BaseTokenIsToken0 {
			baseTokenIsToken0 = 1
		}

		//output :SwapName , liq, tokenPrice, base_token_price

		pairAtDB = &solmodel.Pair{
			ChainId: chainInt,
			Address: trade.PairAddr,
			Name:    constant.PumpFunName,

			FactoryAddress:         "",
			BaseTokenAddress:       trade.PairInfo.BaseTokenAddr,
			TokenAddress:           trade.PairInfo.TokenAddr,
			BaseTokenSymbol:        "SOL",
			TokenSymbol:            tokenSymbol,
			BaseTokenDecimal:       int64(trade.PairInfo.BaseTokenDecimal),
			TokenDecimal:           int64(trade.PairInfo.TokenDecimal),
			BaseTokenIsNativeToken: baseTokenIsNativeToken,
			BaseTokenIsToken0:      baseTokenIsToken0,
			CurrentBaseTokenAmount: trade.CurrentBaseTokenInPoolAmount,
			CurrentTokenAmount:     trade.CurrentTokenInPoolAmount,
			Fdv:                    tokenTotalSupply * trade.TokenPriceUSD,
			MktCap:                 tokenTotalSupply * trade.TokenPriceUSD,
			Liquidity:              liq,
			TokenPrice:             trade.TokenPriceUSD,
			BaseTokenPrice:         trade.BaseTokenPriceUSD,
			BlockNum:               trade.PairInfo.BlockNum,
			BlockTime:              time.Unix(trade.PairInfo.BlockTime, 0),
			Slot:                   trade.Slot,
			// PumpPoint:                    trade.PumpPoint,
			// PumpLaunched:                 BoolToInt64(trade.PumpLaunched),
			// PumpMarketCap:                trade.PumpMarketCap,
			// PumpOwner:                    trade.PumpOwner,
			// PumpSwapPairAddr:             trade.PumpSwapPairAddr,
			// PumpVirtualBaseTokenReserves: trade.PumpVirtualBaseTokenReserves,
			// PumpVirtualTokenReserves:     trade.PumpVirtualTokenReserves,
			// PumpStatus:                   int64(trade.PumpStatus),
			// PumpPairAddr:                 trade.PumpPairAddr,
			LatestTradeTime: time.Unix(trade.PairInfo.BlockTime, 0),
		}

		trade.Mcap = pairAtDB.MktCap
		trade.Fdv = pairAtDB.Fdv

		if trade.PairInfo.InitBaseTokenAmount > 0 && trade.PairInfo.InitTokenAmount > 0 {
			pairAtDB.InitBaseTokenAmount = trade.PairInfo.InitBaseTokenAmount
			pairAtDB.InitTokenAmount = trade.PairInfo.InitTokenAmount
		}

		err = t.sc.PairModel.Insert(ctx, pairAtDB)
		if err != nil {
			if strings.Contains(err.Error(), "Duplicate entry") {
				// db already exists
				pairAtDB, err = t.sc.PairModel.FindOneByChainIdAddress(ctx, chainInt, trade.PairAddr)
				if err != nil {
					return nil, err
				}
				return pairAtDB, nil
			}
			err = fmt.Errorf("PairModel.Insert err:%w", err)
			return nil, err
		}
		t.Infof("pairAtDB insert successful, pairAtDB:%#v", pairAtDB)
		// TODO: add token cache
		// token.TokenSnapCache.Update(int(100000), pairAtDB.TokenAddress, trade.TokenPriceUSD)
	case err == nil:
	default:
		err = fmt.Errorf("PairModel.FindOneByChainIdAddress err:%w", err)
		return nil, err
	}
	// logx.Infof("SavePair:%v db token price: %v, trade token price: %v", trade.PairAddr, pairAtDB.TokenPrice, trade.TokenPriceUSD)

	// 默认值
	trade.Mcap = pairAtDB.MktCap
	trade.Fdv = pairAtDB.Fdv

	//trade存储的是当前交易的快照，保证了实时性，不允许旧数据覆盖覆盖
	if trade.Slot > pairAtDB.Slot {
		// s.Infof("SavePair will UpdatePairDBPoint slot: %v, db slot: %v, hash: %v, pair address: %v", trade.Slot, pairAtDB.Slot, trade.TxHash, trade.PairAddr)

		if pairAtDB.InitBaseTokenAmount == 0 || pairAtDB.InitTokenAmount == 0 {
			if trade.PairInfo.InitBaseTokenAmount > 0 && trade.PairInfo.InitTokenAmount > 0 {
				pairAtDB.InitBaseTokenAmount = trade.PairInfo.InitBaseTokenAmount
				pairAtDB.InitTokenAmount = trade.PairInfo.InitTokenAmount
			}
		}

		// s.initAmount(pairAtDB)

		//因为当前仅测试了Pumpfun，可以先临时设置固定名
		pairAtDB.Name = constant.PumpFunName
		pairAtDB.TokenSymbol = tokenSymbol
		pairAtDB.Slot = trade.Slot
		pairAtDB.Liquidity = liq
		pairAtDB.BlockNum = trade.PairInfo.BlockNum
		err = UpdatePairDBPoint(trade, pairAtDB, tokenTotalSupply)
		if err != nil {
			t.Errorf("SavePair:UpdatePairDBPoint err is %w", err)
		}

		trade.Mcap = pairAtDB.MktCap
		trade.Fdv = pairAtDB.Fdv

		err = t.sc.PairModel.Update(ctx, pairAtDB)
		if err != nil {
			err = fmt.Errorf("PairModel.Update err:%w", err)
			t.Errorf("SavePair:UpdatePairDBPoint err is %w", err)
		}
		t.Infof("pairAtDB update successful, pairAtDB:%#v", pairAtDB)
	}

	return
}

func UpdatePairDBPoint(trade *entity.TradeWithPair, pairDB *solmodel.Pair, tokenTotalSupply float64) error {
	currentTokenInPoolAmount := trade.CurrentTokenInPoolAmount
	currentBaseTokenInPoolAmount := trade.CurrentBaseTokenInPoolAmount
	baseTokenPriceUSD := trade.BaseTokenPriceUSD
	tokenPriceUSD := trade.TokenPriceUSD
	tradeTime := trade.BlockTime

	if pairDB.InitTokenAmount == 0 || pairDB.InitBaseTokenAmount == 0 {
		if trade.PairInfo.InitTokenAmount > 0 && trade.PairInfo.InitBaseTokenAmount > 0 {
			pairDB.InitTokenAmount = trade.PairInfo.InitTokenAmount
			pairDB.InitBaseTokenAmount = trade.PairInfo.InitBaseTokenAmount
			logx.Infof("UpdatePairDBPoint:update init token amount,swapName: %v, %v,%v", trade.SwapName, pairDB.InitTokenAmount, pairDB.InitBaseTokenAmount)
		}
	}

	// pairDB.PumpPoint = trade.PumpPoint
	// pairDB.PumpStatus = int64(trade.PumpStatus)
	// pairDB.PumpVirtualBaseTokenReserves = trade.PumpVirtualBaseTokenReserves
	// pairDB.PumpVirtualTokenReserves = trade.PumpVirtualTokenReserves
	// logx.Infof("UpdatePairDBPoint:update token address: %v pump ponit: %v", trade.PairInfo.TokenAddr, pairDB.PumpPoint)

	// Reset token price if base token liquidity is critically low, unless from specific swap types.
	// if trade.SwapName != util.SwapNamePump && currentBaseTokenInPoolAmount > 0 && currentBaseTokenInPoolAmount < 0.01 {
	// 	tokenPriceUSD = 0
	// }

	// Return early if the trade is older than the last update.
	// if tradeTime < pairDB.LatestTradeTime.Unix() {
	// 	return nil
	// }

	// Update token and base token prices only if valid.
	if tokenPriceUSD > 0 {
		pairDB.TokenPrice = tokenPriceUSD
		// logx.Infof("UpdatePairDBPoint %v db price:%v, trade price %v,", pairDB.Address, pairDB.TokenPrice, trade.TokenPriceUSD)
		// if trade.TokenPriceUSD != pairDB.TokenPrice {
		// 	logx.Infof("Diff UpdatePairDBPoint %v db price:%v, trade price %v,", pairDB.Address, pairDB.TokenPrice, trade.TokenPriceUSD)
		// }
	}
	pairDB.BaseTokenPrice = baseTokenPriceUSD

	// Update FDV (fully diluted valuation) based on token supply.
	if tokenTotalSupply > 0 {
		pairDB.Fdv = decimal.NewFromFloat(tokenPriceUSD).Mul(decimal.NewFromFloat(tokenTotalSupply)).InexactFloat64()
		pairDB.MktCap = decimal.NewFromFloat(tokenPriceUSD).Mul(decimal.NewFromFloat(tokenTotalSupply)).InexactFloat64()
	}

	// Update current liquidity only if both amounts are positive.
	if currentBaseTokenInPoolAmount > 0 && currentTokenInPoolAmount > 0 {
		pairDB.CurrentBaseTokenAmount = currentBaseTokenInPoolAmount
		pairDB.CurrentTokenAmount = currentTokenInPoolAmount
	}

	// Update the latest trade time.
	pairDB.LatestTradeTime = time.Unix(tradeTime, 0)

	// Calculate market cap based on the current liquidity and prices.
	if pairDB.Name == constant.PumpFunName {
		pairDB.Liquidity = decimal.NewFromFloat(baseTokenPriceUSD).Mul(decimal.NewFromFloat(pairDB.CurrentBaseTokenAmount)).Mul(decimal.NewFromFloat(2)).InexactFloat64()
	} else {
		pairDB.Liquidity = decimal.NewFromFloat(tokenPriceUSD).Mul(decimal.NewFromFloat(pairDB.CurrentTokenAmount)).
			Add(decimal.NewFromFloat(baseTokenPriceUSD).Mul(decimal.NewFromFloat(pairDB.CurrentBaseTokenAmount))).InexactFloat64()
	}

	// pairDB.MktCap = tokenPriceUSD*pairDB.CurrentTokenAmount + baseTokenPriceUSD*pairDB.CurrentBaseTokenAmount

	// TODO: Update pair cache.
	// pair.PairCache.Update(pairDB)
	return nil
}

func (t *TradeServiceImpl) UpdateTrade() error {
	return nil
}

func (t *TradeServiceImpl) Stop() {
	t.Info("stop trade service")
	t.cancel(context.Canceled)
}

func NewTradeService(sc *svc.ServiceContext) TradeService {
	ctx, cancel := context.WithCancelCause(context.Background())

	return &TradeServiceImpl{
		ctx:          ctx,
		cancel:       cancel,
		sc:           sc,
		Logger:       logx.WithContext(ctx).WithFields(logx.Field("service", "trade")),
		tokenService: NewTokenService(sc),
	}
}
