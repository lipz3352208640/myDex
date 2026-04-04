package block

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"myDex/myConsumer/internal/logic/entity"
	"myDex/pkg/constant"
	"strings"

	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/near/borsh-go"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
)

type PumpFunService struct {
	ctx context.Context

	cancle func(err error)

	logx.Logger
}

func NewPumpFunService() *PumpFunService {
	ctx, cancle := context.WithCancelCause(context.Background())

	return &PumpFunService{

		ctx: ctx,

		cancle: cancle,

		Logger: logx.WithContext(ctx).WithFields(logx.Field("service", "pumpfun")),
	}
}

// 解析pump指令
func (p *PumpFunService) DecodePumpTranscation(txDecode *entity.TxDecodeEntity) float64 {

	if txDecode.Instruction == nil {
		return 0
	}

	instType, err := GetInstructionType(txDecode.Instruction.Data)
	if err != nil {
		p.Infof("decode instruction type is failed, err = %v", err)
		return 0
	}
	//如果是买卖指令，从meta log中解析出event事件
	if instType == constant.PumpBuyInstruction || instType == constant.PumpSellInstruction {

		price := txDecode.Price
		tokenAccountMap := txDecode.TokenAccountMap
		signature := txDecode.Signature

		if len(txDecode.Instruction.Accounts) <= 5 {
			p.Errorf("pump instruction accounts not enough, signature=%s len=%d", signature, len(txDecode.Instruction.Accounts))
			return 0
		}
		if len(txDecode.AccountKeys) == 0 {
			p.Errorf("pump account keys is empty, signature=%s", signature)
			return 0
		}
		tokenAccountIndex := int(txDecode.Instruction.Accounts[5])
		if tokenAccountIndex >= len(txDecode.AccountKeys) {
			p.Errorf("pump token account index out of range, signature=%s index=%d accountKeys=%d",
				signature, tokenAccountIndex, len(txDecode.AccountKeys))
			return 0
		}

		//获取用户的token account
		tokenAccount := tokenAccountMap[txDecode.AccountKeys[tokenAccountIndex].String()]
		if tokenAccount == nil {
			p.Errorf("pump token account not found, signature=%s tokenAccount=%s",
				signature, txDecode.AccountKeys[tokenAccountIndex].String())
			return 0
		}

		//pump中的曲线状态账户，记录曲线变化的
		curve := txDecode.AccountKeys[txDecode.Instruction.Accounts[3]].String()

		var event *entity.PumpEvent

		fmt.Println("这是pump", len(txDecode.PumpEvents), txDecode.PumpEventIndex)

		if len(txDecode.PumpEvents) == 0 {
			events := p.DecodePumpEvents(txDecode.TranscationMeta.LogMessages)
			evData, _ := json.Marshal(events)
			fmt.Println("this trade event is ", string(evData))
			if events == nil {
				return 0
			}
			txDecode.PumpEvents = events
			if txDecode.PumpEventIndex >= len(events) {
				p.Errorf("pump event index out of range after decode, signature=%s index=%d events=%d",
					signature, txDecode.PumpEventIndex, len(events))
				return 0
			}
			event = events[txDecode.PumpEventIndex]

		} else {
			if txDecode.PumpEventIndex >= len(txDecode.PumpEvents) {
				return 0
			}
			event = txDecode.PumpEvents[txDecode.PumpEventIndex]
		}

		txDecode.PumpEventIndex++

		solAmount := event.SolAmount
		tokenAmount := event.TokenAccount

		fmt.Printf("pump debug: solUsd=%f, solAmount=%d, tokenAmount=%d, tokenDecimal=%d\n",
			price, event.SolAmount, event.TokenAccount, tokenAccount.TokenDecimal)

		//链上的原始数量转换为实际数量
		realSolAccount := decimal.New(int64(solAmount), -constant.SolDecimal).InexactFloat64()
		realTokenAccount := decimal.New(int64(tokenAmount), -int32(tokenAccount.TokenDecimal)).InexactFloat64()
		if realTokenAccount == 0 {
			p.Errorf("pump real token account is zero, signature=%s", signature)
			return 0
		}

		//计算token price
		totalTokenAccount := decimal.NewFromFloat(realSolAccount).Mul(decimal.NewFromFloat(price)).InexactFloat64()
		tokenPrice := decimal.NewFromFloat(totalTokenAccount).Div(decimal.NewFromFloat(realTokenAccount)).InexactFloat64()

		p.Infof("this transaction signature is %s, price is %f", signature, tokenPrice)

		//获取流动性池子中token的流动性

		//const TokenReservesDiff = 279900000000000 // Token虚拟储备量 - Token实际储备量
		realTokenReserves := event.VirtualTokenReserves - constant.TokenReservesDiff
		realSolReserves := event.VirtualSolReserves - constant.SolReservesDiff

		fmt.Println("token真实流动性：", realTokenReserves)
		fmt.Println("sol真实流动性：", realSolReserves)
		fmt.Println("pump曲线状态：", curve)

		return tokenPrice
	}
	return 0
}

// 解析pump event事件
func (p *PumpFunService) DecodePumpEvents(logs []string) (events []*entity.PumpEvent) {

	lo.ForEach(logs, func(log string, index int) {

		fmt.Printf("log is: %s, %d\n", log, index)

		if len(log) > 100 && strings.HasPrefix(log, "Program data: vdt/007") {
			logData := strings.TrimPrefix(log, "Program data: ")

			var decodeDataByte []byte
			//先尝试用base64解码
			if eventBase64Binary, err := base64.StdEncoding.DecodeString(logData); err != nil {
				eventBase64RawBinary, err := base64.RawStdEncoding.DecodeString(logData)
				if err != nil {
					p.Errorf("base64 decode failed for both StdEncoding and RawStdEncoding: %w", err)
					return
				} else {
					decodeDataByte = eventBase64RawBinary
				}
			} else {
				decodeDataByte = eventBase64Binary
			}

			var pumpEvent entity.PumpEvent

			err := borsh.Deserialize(&pumpEvent, decodeDataByte)
			if err == nil {
				events = append(events, &pumpEvent)
			}
		}
	})
	return events
}

func GetInstructionType(data []byte) (uint64, error) {

	if len(data) < 8 {
		return 0, fmt.Errorf("invalid instruction data length: %d", len(data))
	}
	return binary.LittleEndian.Uint64(data[:8]), nil
}

func (p *PumpFunService) ParsePumpInstruction(header types.MessageHeader, accountKeys []common.PublicKey, instruction types.CompiledInstruction) (string, error) {
	data := instruction.Data
	if len(data) < 8 {
		return "", fmt.Errorf("invalid instruction data length: %d", len(data))
	}
	instType := binary.LittleEndian.Uint64(data[:8])
	switch instType {
	case constant.PumpBuyInstruction:
		pumpBuyInstruction := ParsePumpBuyInstruction(header, accountKeys, instruction)
		data, _ := json.Marshal(pumpBuyInstruction)
		return string(data), nil
	case constant.PumpSellInstruction:
		pumpSellInstruction := ParsePumpSellInstruction(header, accountKeys, instruction)
		data, _ := json.Marshal(pumpSellInstruction)
		return string(data), nil
	default:
		return "", fmt.Errorf("unknown instruction type: %d", instType)
	}
}

func (p *PumpFunService) ParsePumpBuyInstruction(header types.MessageHeader, accountKeys []common.PublicKey, instruction types.CompiledInstruction) entity.BuyInstruction {
	// 解析买入指令账户
	pool := accountKeys[instruction.Accounts[0]].String()
	user := accountKeys[instruction.Accounts[1]].String()
	globalConfig := accountKeys[instruction.Accounts[2]].String()
	baseMint := accountKeys[instruction.Accounts[3]].String()
	quoteMint := accountKeys[instruction.Accounts[4]].String()
	userBaseTokenAccount := accountKeys[instruction.Accounts[5]].String()
	userQuoteTokenAccount := accountKeys[instruction.Accounts[6]].String()
	poolBaseTokenAccount := accountKeys[instruction.Accounts[7]].String()
	poolQuoteTokenAccount := accountKeys[instruction.Accounts[8]].String()
	protocolFeeRecipient := accountKeys[instruction.Accounts[9]].String()
	protocolFeeRecipientTokenAccount := accountKeys[instruction.Accounts[10]].String()
	baseTokenProgram := accountKeys[instruction.Accounts[11]].String()
	quoteTokenProgram := accountKeys[instruction.Accounts[12]].String()
	systemProgram := accountKeys[instruction.Accounts[13]].String()
	associatedTokenProgram := accountKeys[instruction.Accounts[14]].String()
	eventAuthority := accountKeys[instruction.Accounts[15]].String()
	program := accountKeys[instruction.Accounts[16]].String()
	coinCreatorVaultAta := accountKeys[instruction.Accounts[17]].String()
	coinCreatorVaultAuthority := accountKeys[instruction.Accounts[18]].String()
	globalVolumeAccumulator := accountKeys[instruction.Accounts[19]].String()
	userVolumeAccumulator := accountKeys[instruction.Accounts[20]].String()
	feeConfig := accountKeys[instruction.Accounts[21]].String()
	feeProgram := accountKeys[instruction.Accounts[22]].String()

	//解析买入的指令data
	baseAmountOut := binary.LittleEndian.Uint64(instruction.Data[8:16])
	maxQuoteAmountIn := binary.LittleEndian.Uint64(instruction.Data[16:24])
	trackVolume := instruction.Data[24] == 1

	return entity.BuyInstruction{
		ProgranId: accountKeys[instruction.ProgramIDIndex].String(),
		Accounts: entity.PumpBuyAccount{
			Pool:                             makeSignerAccount(pool, instruction.Accounts[0], len(accountKeys), header),
			User:                             makeSignerAccount(user, instruction.Accounts[1], len(accountKeys), header),
			GlobalConfig:                     makeSignerAccount(globalConfig, instruction.Accounts[2], len(accountKeys), header),
			BaseMint:                         makeSignerAccount(baseMint, instruction.Accounts[3], len(accountKeys), header),
			QuoteMint:                        makeSignerAccount(quoteMint, instruction.Accounts[4], len(accountKeys), header),
			UserBaseTokenAccount:             makeSignerAccount(userBaseTokenAccount, instruction.Accounts[5], len(accountKeys), header),
			UserQuoteTokenAccount:            makeSignerAccount(userQuoteTokenAccount, instruction.Accounts[6], len(accountKeys), header),
			PoolBaseTokenAccount:             makeSignerAccount(poolBaseTokenAccount, instruction.Accounts[7], len(accountKeys), header),
			PoolQuoteTokenAccount:            makeSignerAccount(poolQuoteTokenAccount, instruction.Accounts[8], len(accountKeys), header),
			ProtocolFeeRecipient:             makeSignerAccount(protocolFeeRecipient, instruction.Accounts[9], len(accountKeys), header),
			ProtocolFeeRecipientTokenAccount: makeSignerAccount(protocolFeeRecipientTokenAccount, instruction.Accounts[10], len(accountKeys), header),
			BaseTokenProgram:                 makeSignerAccount(baseTokenProgram, instruction.Accounts[11], len(accountKeys), header),
			QuoteTokenProgram:                makeSignerAccount(quoteTokenProgram, instruction.Accounts[12], len(accountKeys), header),
			SystemProgram:                    makeSignerAccount(systemProgram, instruction.Accounts[13], len(accountKeys), header),
			AssociatedTokenProgram:           makeSignerAccount(associatedTokenProgram, instruction.Accounts[14], len(accountKeys), header),
			EventAuthority:                   makeSignerAccount(eventAuthority, instruction.Accounts[15], len(accountKeys), header),
			Program:                          makeSignerAccount(program, instruction.Accounts[16], len(accountKeys), header),
			CoinCreatorVaultAta:              makeSignerAccount(coinCreatorVaultAta, instruction.Accounts[17], len(accountKeys), header),
			CoinCreatorVaultAuthority:        makeSignerAccount(coinCreatorVaultAuthority, instruction.Accounts[18], len(accountKeys), header),
			GlobalVolumeAccumulator:          makeSignerAccount(globalVolumeAccumulator, instruction.Accounts[19], len(accountKeys), header),
			UserVolumeAccumulator:            makeSignerAccount(userVolumeAccumulator, instruction.Accounts[20], len(accountKeys), header),
			FeeConfig:                        makeSignerAccount(feeConfig, instruction.Accounts[21], len(accountKeys), header),
			FeeProgram:                       makeSignerAccount(feeProgram, instruction.Accounts[22], len(accountKeys), header),
		},
		Data: entity.PumpBuyData{
			BaseAmountOut:    baseAmountOut,
			MaxQuoteAmountIn: maxQuoteAmountIn,
			TrackVolume:      trackVolume,
		},
	}
}

func (p *PumpFunService) ParsePumpSellInstruction(header types.MessageHeader, accountKeys []common.PublicKey, instruction types.CompiledInstruction) entity.SellInstruction {
	// 解析买入指令账户
	pool := accountKeys[instruction.Accounts[0]].String()
	user := accountKeys[instruction.Accounts[1]].String()
	globalConfig := accountKeys[instruction.Accounts[2]].String()
	baseMint := accountKeys[instruction.Accounts[3]].String()
	quoteMint := accountKeys[instruction.Accounts[4]].String()
	userBaseTokenAccount := accountKeys[instruction.Accounts[5]].String()
	userQuoteTokenAccount := accountKeys[instruction.Accounts[6]].String()
	poolBaseTokenAccount := accountKeys[instruction.Accounts[7]].String()
	poolQuoteTokenAccount := accountKeys[instruction.Accounts[8]].String()
	protocolFeeRecipient := accountKeys[instruction.Accounts[9]].String()
	protocolFeeRecipientTokenAccount := accountKeys[instruction.Accounts[10]].String()
	baseTokenProgram := accountKeys[instruction.Accounts[11]].String()
	quoteTokenProgram := accountKeys[instruction.Accounts[12]].String()
	systemProgram := accountKeys[instruction.Accounts[13]].String()
	associatedTokenProgram := accountKeys[instruction.Accounts[14]].String()
	eventAuthority := accountKeys[instruction.Accounts[15]].String()
	program := accountKeys[instruction.Accounts[16]].String()
	coinCreatorVaultAta := accountKeys[instruction.Accounts[17]].String()
	coinCreatorVaultAuthority := accountKeys[instruction.Accounts[18]].String()
	feeConfig := accountKeys[instruction.Accounts[19]].String()
	feeProgram := accountKeys[instruction.Accounts[20]].String()

	//解析卖出的指令data
	baseAmountOut := binary.LittleEndian.Uint64(instruction.Data[8:16])
	minQuoteAmountOut := binary.LittleEndian.Uint64(instruction.Data[16:24])

	return entity.SellInstruction{
		ProgranId: accountKeys[instruction.ProgramIDIndex].String(),
		Accounts: entity.PumpSellAccount{
			Pool:                             makeSignerAccount(pool, instruction.Accounts[0], len(accountKeys), header),
			User:                             makeSignerAccount(user, instruction.Accounts[1], len(accountKeys), header),
			GlobalConfig:                     makeSignerAccount(globalConfig, instruction.Accounts[2], len(accountKeys), header),
			BaseMint:                         makeSignerAccount(baseMint, instruction.Accounts[3], len(accountKeys), header),
			QuoteMint:                        makeSignerAccount(quoteMint, instruction.Accounts[4], len(accountKeys), header),
			UserBaseTokenAccount:             makeSignerAccount(userBaseTokenAccount, instruction.Accounts[5], len(accountKeys), header),
			UserQuoteTokenAccount:            makeSignerAccount(userQuoteTokenAccount, instruction.Accounts[6], len(accountKeys), header),
			PoolBaseTokenAccount:             makeSignerAccount(poolBaseTokenAccount, instruction.Accounts[7], len(accountKeys), header),
			PoolQuoteTokenAccount:            makeSignerAccount(poolQuoteTokenAccount, instruction.Accounts[8], len(accountKeys), header),
			ProtocolFeeRecipient:             makeSignerAccount(protocolFeeRecipient, instruction.Accounts[9], len(accountKeys), header),
			ProtocolFeeRecipientTokenAccount: makeSignerAccount(protocolFeeRecipientTokenAccount, instruction.Accounts[10], len(accountKeys), header),
			BaseTokenProgram:                 makeSignerAccount(baseTokenProgram, instruction.Accounts[11], len(accountKeys), header),
			QuoteTokenProgram:                makeSignerAccount(quoteTokenProgram, instruction.Accounts[12], len(accountKeys), header),
			SystemProgram:                    makeSignerAccount(systemProgram, instruction.Accounts[13], len(accountKeys), header),
			AssociatedTokenProgram:           makeSignerAccount(associatedTokenProgram, instruction.Accounts[14], len(accountKeys), header),
			EventAuthority:                   makeSignerAccount(eventAuthority, instruction.Accounts[15], len(accountKeys), header),
			Program:                          makeSignerAccount(program, instruction.Accounts[16], len(accountKeys), header),
			CoinCreatorVaultAta:              makeSignerAccount(coinCreatorVaultAta, instruction.Accounts[17], len(accountKeys), header),
			CoinCreatorVaultAuthority:        makeSignerAccount(coinCreatorVaultAuthority, instruction.Accounts[18], len(accountKeys), header),
			FeeConfig:                        makeSignerAccount(feeConfig, instruction.Accounts[19], len(accountKeys), header),
			FeeProgram:                       makeSignerAccount(feeProgram, instruction.Accounts[20], len(accountKeys), header),
		},
		Data: entity.PumpSellData{
			BaseAmountOut:     baseAmountOut,
			MinQuoteAmountOut: minQuoteAmountOut,
		},
	}
}

func (p *PumpFunService) makeSignerAccount(address string,
	index int,
	length int,
	header types.MessageHeader) entity.Account {

	//计算签名，可写权限的位置
	readOnlySigner := header.NumReadonlySignedAccounts
	signer := header.NumRequireSignatures
	readOnlyUnsigner := header.NumReadonlyUnsignedAccounts

	var isWritable bool = false
	var isSigner bool = false

	if uint8(index) < signer-readOnlySigner {
		isWritable = true
		isSigner = true
	}

	if uint8(index) >= signer-readOnlySigner && uint8(index) < signer {
		isWritable = false
		isSigner = true
	}

	if uint8(index) >= signer && uint8(index) < uint8(length)-readOnlyUnsigner {
		isWritable = true
		isSigner = false
	}

	if uint8(index) >= uint8(length)-readOnlyUnsigner && uint8(index) < uint8(length) {
		isWritable = false
		isSigner = false
	}
	return entity.Account{
		Address:    address,
		IsWritable: isWritable,
		IsSigner:   isSigner,
	}

}
