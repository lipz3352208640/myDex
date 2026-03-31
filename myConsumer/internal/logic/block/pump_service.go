package block

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"myDex/myConsumer/internal/logic/entity"
	"myDex/pkg/constant"

	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
)

func ParsePumpInstruction(header types.MessageHeader, accountKeys []common.PublicKey, instruction types.CompiledInstruction) (string, error) {
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

func ParsePumpBuyInstruction(header types.MessageHeader, accountKeys []common.PublicKey, instruction types.CompiledInstruction) entity.BuyInstruction {
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

func ParsePumpSellInstruction(header types.MessageHeader, accountKeys []common.PublicKey, instruction types.CompiledInstruction) entity.SellInstruction {
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

func makeSignerAccount(address string,
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
