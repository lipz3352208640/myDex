package block

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"myDex/model/solmodel"
	"myDex/myConsumer/internal/logic/entity"
	"myDex/pkg/constant"
	"strconv"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/mr-tron/base58"
	"github.com/samber/lo"
)

var ProgramOrca = common.PublicKeyFromString("whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc")
var ProgramRaydiumConcentratedLiquidity = common.PublicKeyFromString("CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK")
var ProgramMeteoraDLMM = common.PublicKeyFromString("LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo")
var ProgramPhoneNix = common.PublicKeyFromString("PhoeNiXZ8ByJGLkxNfZRnkUfjvmuYqLR89jjFHGqdXY")

var StableCoinSwapDexes = []common.PublicKey{ProgramOrca, ProgramRaydiumConcentratedLiquidity, ProgramMeteoraDLMM, ProgramPhoneNix}

func GetSolPrice(block *client.Block, dbBlock *solmodel.Block, b *BlockService) (map[string]*entity.TokenAccount, float64) {
	var prices []float64
	tokenAccountMap := make(map[string]*entity.TokenAccount)
	for _, transcation := range block.Transactions {
		innerInstMap := make(map[int]*client.InnerInstruction)
		makeInnerInstWithOuterMap(transcation, innerInstMap)
		hasChange := FillTokenAccountMap(transcation, tokenAccountMap)
		if !hasChange {
			continue
		}

		priceList := GetSolPriceBySwap(innerInstMap, tokenAccountMap, transcation)
		prices = append(prices, priceList...)
		if len(priceList) > 0 {
			signature := getTransactionSignature(transcation)
			fmt.Printf("sol price tx trace: slot=%d signature=%s prices=%v url=https://solscan.io/tx/%s\n",
				dbBlock.Slot, signature, priceList, signature)
		}
	}

	finalPrice := RemoveMinAndMaxAndCalculateAverage(prices)
	if finalPrice > 0 {
		return tokenAccountMap, finalPrice
	}
	if dbBlock.SolPrice > 0 {
		return tokenAccountMap, dbBlock.SolPrice
	}
	if block, err := b.sc.BlockModel.FindOneNearSlot(b.ctx, dbBlock.Slot); err == nil && block != nil {
		return tokenAccountMap, block.SolPrice
	}
	return tokenAccountMap, 0

}

func getTransactionSignature(transcation client.BlockTransaction) string {
	if len(transcation.Transaction.Signatures) == 0 {
		return ""
	}
	return base58.Encode(transcation.Transaction.Signatures[0])
}

func GetSolPriceBySwap(innerInstMap map[int]*client.InnerInstruction,
	tokenAccountMap map[string]*entity.TokenAccount,
	transcation client.BlockTransaction) []float64 {
	accountKeys := transcation.AccountKeys
	//signature := getTransactionSignature(transcation)

	var prices []float64
	lo.ForEach(transcation.Transaction.Message.Instructions, func(inst types.CompiledInstruction, index int) {
		if lo.Contains(StableCoinSwapDexes, accountKeys[inst.ProgramIDIndex]) {
			if innerInstMap[index] == nil {
				return
			}

			var allTransfer []*token.TransferParam

			lo.ForEach(innerInstMap[index].Instructions, func(inst types.CompiledInstruction, innerIndex int) {
				transfer, err := getTransfer(accountKeys, inst)
				if err != nil || transfer == nil {
					return
				}
				from := tokenAccountMap[transfer.From.String()]
				to := tokenAccountMap[transfer.To.String()]

				if from != nil && to != nil {
					if from.TokenMintAccountAddress == to.TokenMintAccountAddress {

						if from.TokenMintAccountAddress == constant.Wsol || from.TokenMintAccountAddress == constant.Usdc {
							allTransfer = append(allTransfer, transfer)
							// fmt.Printf("sol price debug: signature=%s outerIndex=%d innerIndex=%d mint=%s amount=%d from=%s to=%s fromOwner=%s toOwner=%s\n",
							// 	signature,
							// 	index,
							// 	innerIndex,
							// 	from.TokenMintAccountAddress,
							// 	transfer.Amount,
							// 	transfer.From.String(),
							// 	transfer.To.String(),
							// 	from.Owner,
							// 	to.Owner,
							// )
						}
					}
				}

			})

			//swapTransfer分组并计算价格
			price := CalcPriceOnAllTransferBySwapGroup(allTransfer, tokenAccountMap)
			// fmt.Printf("sol price debug: signature=%s outerIndex=%d calculatedPrice=%f\n",
			// 	signature, index, price)
			if price > 0 {
				prices = append(prices, price)
			}
		}
	})

	lo.ForEach(transcation.Meta.InnerInstructions, func(inst client.InnerInstruction, index int) {
		lo.ForEach(inst.Instructions, func(innerInst types.CompiledInstruction, i int) {
			if lo.Contains(StableCoinSwapDexes, accountKeys[innerInst.ProgramIDIndex]) {

				innerInsruction := GetInnerInstructionByInner(inst.Instructions, i, 2)
				if innerInsruction == nil {
					return
				}

				var allTransfer []*token.TransferParam

				lo.ForEach(innerInsruction.Instructions, func(inst types.CompiledInstruction, innerIndex int) {
					transfer, err := getTransfer(accountKeys, inst)
					if err != nil || transfer == nil {
						return
					}
					from := tokenAccountMap[transfer.From.String()]
					to := tokenAccountMap[transfer.To.String()]

					if from != nil && to != nil {
						if from.TokenMintAccountAddress == to.TokenMintAccountAddress {

							if from.TokenMintAccountAddress == constant.Wsol || from.TokenMintAccountAddress == constant.Usdc {
								allTransfer = append(allTransfer, transfer)
								// fmt.Printf("sol price debug: signature=%s outerIndex=%d innerIndex=%d mint=%s amount=%d from=%s to=%s fromOwner=%s toOwner=%s\n",
								// 	signature,
								// 	index,
								// 	innerIndex,
								// 	from.TokenMintAccountAddress,
								// 	transfer.Amount,
								// 	transfer.From.String(),
								// 	transfer.To.String(),
								// 	from.Owner,
								// 	to.Owner,
								// )
							}
						}
					}

				})

				//swapTransfer分组并计算价格
				price := CalcPriceOnAllTransferBySwapGroup(allTransfer, tokenAccountMap)
				// fmt.Printf("sol price debug: signature=%s outerIndex=%d calculatedPrice=%f\n",
				// 	signature, index, price)
				if price > 0 {
					prices = append(prices, price)
				}
			}
		})
	})

	return prices

}

func RemoveMinAndMaxAndCalculateAverage(nums []float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	if len(nums) == 1 {
		return nums[0]
	}
	if len(nums) == 2 {
		return (nums[0] + nums[1]) / 2
	}

	minVal, maxVal := math.MaxFloat64, -math.MaxFloat64
	minIndex, maxIndex := -1, -1

	for i, num := range nums {
		if num < minVal {
			minVal = num
			minIndex = i
		}
		if num > maxVal {
			maxVal = num
			maxIndex = i
		}
	}

	var filteredNums []float64
	for i, num := range nums {
		if i != minIndex && i != maxIndex {
			filteredNums = append(filteredNums, num)
		}
	}

	sum := 0.0
	for _, num := range filteredNums {
		sum += num
	}
	average := sum / float64(len(filteredNums))

	return average
}

func getTransfer(accountKeys []common.PublicKey, inst types.CompiledInstruction) (*token.TransferParam, error) {
	transfer := &token.TransferParam{}
	if accountKeys[inst.ProgramIDIndex].String() == constant.TokenProgramID ||
		accountKeys[inst.ProgramIDIndex].String() == constant.Token2022ProgramID {
		switch token.Instruction(inst.Data[0]) {
		case token.InstructionTransfer:
			if len(inst.Data) < 9 || len(inst.Accounts) < 3 {
				return nil, errors.New("transfer instruction is error, because account length < 9 and data length < 3")
			}
			transfer.From = accountKeys[inst.Accounts[0]]
			transfer.To = accountKeys[inst.Accounts[1]]
			transfer.Amount = binary.LittleEndian.Uint64(inst.Data[1:])
			return transfer, nil

		case token.InstructionTransferChecked:
			if len(inst.Data) < 10 || len(inst.Accounts) < 4 {
				return nil, errors.New("transfer instruction is error, because account length < 10 and data length < 4")
			}
			transfer.From = accountKeys[inst.Accounts[0]]
			transfer.To = accountKeys[inst.Accounts[2]]
			transfer.Amount = binary.LittleEndian.Uint64(inst.Data[1:10])
			return transfer, nil
		default:
			errors.New("not found transfer instruction")
			return nil, errors.New("not found transfer instruction")
		}

	}
	return nil, nil
}

func CalcPriceOnAllTransferBySwapGroup(allTransfer []*token.TransferParam,
	tokenAccountMap map[string]*entity.TokenAccount) float64 {

	if len(allTransfer) == 0 {
		return 0
	}

	var usdcTotal uint64
	var usdgTotal uint64
	var usdcTokenAccount *entity.TokenAccount
	var usdgTokenAccount *entity.TokenAccount
	use := make([]bool, len(allTransfer))
	for i := 0; i < len(allTransfer); i++ {
		if use[i] {
			continue
		}

		fromTokenAccount := tokenAccountMap[allTransfer[i].From.String()]

		if fromTokenAccount == nil {
			continue
		}

		for j := i + 1; j < len(allTransfer); j++ {
			if use[j] {
				continue
			}
			toTokenAccount := tokenAccountMap[allTransfer[j].From.String()]
			if toTokenAccount == nil {
				continue
			}
			if fromTokenAccount.TokenMintAccountAddress == toTokenAccount.TokenMintAccountAddress {
				continue
			}

			if fromTokenAccount.TokenMintAccountAddress == constant.Wsol {
				if IsSwapTransfer(allTransfer[j], allTransfer[i], tokenAccountMap) {
					usdcTotal += allTransfer[j].Amount
					usdgTotal += allTransfer[i].Amount
					use[i] = true
					use[j] = true
					usdcTokenAccount = toTokenAccount
					usdgTokenAccount = fromTokenAccount
					break
				}
			} else if fromTokenAccount.TokenMintAccountAddress == constant.Usdc {
				if IsSwapTransfer(allTransfer[i], allTransfer[j], tokenAccountMap) {
					usdcTotal += allTransfer[i].Amount
					usdgTotal += allTransfer[j].Amount
					use[i] = true
					use[j] = true
					usdcTokenAccount = fromTokenAccount
					usdgTokenAccount = toTokenAccount
					break
				}
			}
		}
	}
	if usdcTokenAccount == nil || usdgTokenAccount == nil {
		return 0
	}
	usdcUI := float64(usdcTotal) / math.Pow10(int(usdcTokenAccount.TokenDecimal))
	usdgUI := float64(usdgTotal) / math.Pow10(int(usdgTokenAccount.TokenDecimal))
	if usdgUI == 0 {
		return 0
	}
	return usdcUI / usdgUI
}

func IsSwapTransfer(usdcTransfer *token.TransferParam,
	usdgTransfer *token.TransferParam,
	tokenAccountMap map[string]*entity.TokenAccount) bool {

	usdcFromTokenAccount := tokenAccountMap[usdcTransfer.From.String()]
	usdcToTokenAccount := tokenAccountMap[usdcTransfer.To.String()]
	usdgFromTokenAccount := tokenAccountMap[usdgTransfer.From.String()]
	usdgToTokenAccount := tokenAccountMap[usdgTransfer.To.String()]

	//在多条路由存在下，保证单边匹配
	if usdcFromTokenAccount.Owner == usdgToTokenAccount.Owner {
		return true
	}
	if usdcToTokenAccount.Owner == usdgFromTokenAccount.Owner {
		return true
	}
	return false
}

func in(StableCoinSwapDexes []common.PublicKey, accountKeys []common.PublicKey) bool {
	panic("unimplemented")
}

// 建立内外指令之间的映射
func makeInnerInstWithOuterMap(transcation client.BlockTransaction,
	innerMap map[int]*client.InnerInstruction) {
	if len(transcation.Meta.InnerInstructions) == 0 {
		return
	}
	for _, innerInst := range transcation.Meta.InnerInstructions {
		innerMap[int(innerInst.Index)] = &innerInst
	}
}

// 多跳路由情况下，对内层指令构造swap组。一般一个swap指令下面跟的就是两笔transfer
func GetInnerInstructionByInner(instructions []types.CompiledInstruction, startIndex, innerLen int) *client.InnerInstruction {
	if startIndex+innerLen+1 > len(instructions) {
		return nil
	}
	innerInstruction := &client.InnerInstruction{
		Index: uint64(instructions[startIndex].ProgramIDIndex),
	}
	for i := 0; i < innerLen; i++ {
		innerInstruction.Instructions = append(innerInstruction.Instructions, instructions[startIndex+i+1])
	}
	return innerInstruction
}

// 填充token账户
func FillTokenAccountMap(transcation client.BlockTransaction,
	tokenAccountMap map[string]*entity.TokenAccount) bool {
	var hasChange bool
	accountKeys := transcation.AccountKeys
	if len(transcation.Meta.PreTokenBalances) != 0 {
		lo.ForEach(transcation.Meta.PreTokenBalances, func(item rpc.TransactionMetaTokenBalance, index int) {
			value, _ := strconv.ParseInt(item.UITokenAmount.Amount, 10, 64)
			if tokenAccountMap[accountKeys[item.AccountIndex].String()] == nil {
				tokenAccountMap[accountKeys[item.AccountIndex].String()] = &entity.TokenAccount{
					Owner:                   item.Owner,
					PreValue:                value,
					PreValueUIString:        item.UITokenAmount.UIAmountString,
					TokenDecimal:            item.UITokenAmount.Decimals,
					TokenMintAccountAddress: item.Mint,
					TokenAccountAddress:     accountKeys[item.AccountIndex].String(),
					Closed:                  true,
					Init:                    true,
				}
			}
		})
	}

	if len(transcation.Meta.PostTokenBalances) != 0 {
		lo.ForEach(transcation.Meta.PostTokenBalances, func(item rpc.TransactionMetaTokenBalance, index int) {
			value, _ := strconv.ParseInt(item.UITokenAmount.Amount, 10, 64)

			if tokenAccountMap[accountKeys[item.AccountIndex].String()] != nil {
				tokenAccount := tokenAccountMap[accountKeys[item.AccountIndex].String()]
				tokenAccount.Closed = false
				tokenAccount.PostValue = value
				tokenAccount.PostValueUIString = item.UITokenAmount.UIAmountString
				if tokenAccount.PostValue != tokenAccount.PreValue {
					hasChange = true
				}
				if item.UITokenAmount.Decimals != 0 {
					tokenAccount.TokenDecimal = item.UITokenAmount.Decimals
				}

			} else {
				tokenAccountMap[accountKeys[item.AccountIndex].String()] = &entity.TokenAccount{
					Owner:                   item.Owner,
					PostValue:               value,
					PostValueUIString:       item.UITokenAmount.UIAmountString,
					TokenDecimal:            item.UITokenAmount.Decimals,
					TokenMintAccountAddress: item.Mint,
					TokenAccountAddress:     accountKeys[item.AccountIndex].String(),
					Closed:                  false,
					Init:                    true,
				}
			}
		})
	}

	//补充没有金额变动的账户，为了保证账户的完整性(owner,mint等信息)，在进行价格计算保证不会缺少owner等信息
	if len(transcation.Transaction.Message.Instructions) != 0 {

		lo.ForEach(transcation.Transaction.Message.Instructions, func(inst types.CompiledInstruction, index int) {
			programId := accountKeys[inst.ProgramIDIndex].String()
			//判断合约属于TokenProgram或者Token2022Program
			if programId == constant.TokenProgramID || programId == constant.Token2022ProgramID {
				shouldReturn := FilterIntializeAccount(inst, accountKeys, tokenAccountMap)
				if shouldReturn {
					return
				}
			}
		})
	}

	if len(transcation.Meta.InnerInstructions) != 0 {

		lo.ForEach(transcation.Meta.InnerInstructions, func(insts client.InnerInstruction, index int) {
			lo.ForEach(insts.Instructions, func(inst types.CompiledInstruction, index int) {
				programId := accountKeys[inst.ProgramIDIndex].String()
				//判断合约属于TokenProgram或者Token2022Program
				if programId == constant.TokenProgramID || programId == constant.Token2022ProgramID {
					shouldReturn := FilterIntializeAccount(inst, accountKeys, tokenAccountMap)
					if shouldReturn {
						return
					}
				}
			})
		})
	}

	accountDecimaMap := make(map[string]uint8)
	for _, item := range tokenAccountMap {
		accountDecimaMap[item.TokenMintAccountAddress] = item.TokenDecimal
	}
	for _, item := range tokenAccountMap {
		item.TokenDecimal = accountDecimaMap[item.TokenMintAccountAddress]
	}

	return hasChange

}

func FilterIntializeAccount(inst types.CompiledInstruction, accountKeys []common.PublicKey, tokenAccountMap map[string]*entity.TokenAccount) bool {
	var account string
	var mint string
	var owner string
	switch token.Instruction(inst.Data[0]) {
	case token.InstructionInitializeAccount:
		if len(inst.Accounts) < 3 {
			return true
		}
		account = accountKeys[inst.Accounts[0]].String()
		mint = accountKeys[inst.Accounts[1]].String()
		owner = accountKeys[inst.Accounts[2]].String()
	case token.InstructionInitializeAccount2:
		if len(inst.Accounts) < 2 || len(inst.Data) < 33 {
			return true
		}
		account = accountKeys[inst.Accounts[0]].String()
		mint = accountKeys[inst.Accounts[1]].String()
		owner = common.PublicKeyFromBytes(inst.Data[1:]).String()
	case token.InstructionInitializeAccount3:
		if len(inst.Accounts) < 2 || len(inst.Data) < 33 {
			return true
		}
		account = accountKeys[inst.Accounts[0]].String()
		mint = accountKeys[inst.Accounts[1]].String()
		owner = common.PublicKeyFromBytes(inst.Data[1:]).String()
	default:
		return true
	}
	//关注交易账户相关的账户
	if tokenAccountMap[account] != nil && tokenAccountMap[account].TokenMintAccountAddress == mint {
		return true
	}

	tokenAccountMap[account] = &entity.TokenAccount{
		Owner:                   owner,
		TokenAccountAddress:     account,
		TokenMintAccountAddress: mint,
		Closed:                  false,
		Init:                    true,
	}
	return false
}
