package pumpamm

import (
	"context"
	"fmt"
	"myDex/pkg/constant"
	"myDex/pkg/xcode"
	"myDex/trade/internal/chain/solana/entity"
	"myDex/trade/trade"

	token2022 "myDex/pkg/token2022"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	aSDK "github.com/gagliardetto/solana-go"
	ata "github.com/gagliardetto/solana-go/programs/associated-token-account"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
)

type PumpfunAmm struct {
	ctx context.Context
	logx.Logger
	cancle func(err error)
	client *ag_rpc.Client
}

func NewPumpfunAmm(rpcEndpoint string) *PumpfunAmm {
	ctx, cancle := context.WithCancelCause(context.Background())
	return &PumpfunAmm{
		ctx:    ctx,
		cancle: cancle,
		Logger: logx.WithContext(context.Background()).WithFields(logx.LogField{Key: "service", Value: "pumpFunAmm"}),
		client: ag_rpc.New(rpcEndpoint),
	}
}

func (amm *PumpfunAmm) CreateMarketOrderPumpfunAmm(market *entity.MarketTxExt) ([]aSDK.Instruction, error) {

	var instruction []aSDK.Instruction

	amountIn, err := decimal.NewFromString(market.AmountIn)
	if err != nil {
		return nil, fmt.Errorf("amountIn formatter err is: %v", err)
	}

	//step 1: add compute budget instruction
	computeBudgetInstruction, err := amm.CreateGasByGasFee(amm.ctx, false, market.UserWalletAddress, constant.PumpFunSwapCU, constant.GasMODE[1])
	if err != nil {
		return nil, fmt.Errorf("create compute budget instruction err is: %v", err)
	}
	instruction = append(instruction, computeBudgetInstruction...)

	//step 3: check wallet balance is can cover gas fee
	result, err := amm.client.GetBalance(amm.ctx, market.UserWalletAddress, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, fmt.Errorf("get wallet balance err is :%v", err)
	}
	balance := result.Value

	reminBalance := amountIn.Sub(decimal.NewFromUint64(balance))
	if reminBalance.LessThan(decimal.NewFromInt(0)) {
		return nil, xcode.SolBalanceNotEnough
	}
	if market.SwapType == int32(trade.SwapType_Buy) {
		//输入：计算服务费, 总花费=服务费+输入的金额+交易费
		totalAmount := amountIn.Mul(decimal.NewFromInt(1).Add(constant.ServericeFeePercent)).Add(decimal.NewFromUint64(constant.GasMODE[1]))

		remin := decimal.NewFromUint64(balance).Sub(totalAmount)
		if remin.LessThan(decimal.NewFromInt(0)) {
			return nil, xcode.SolBalanceNotEnough
		}
	} else {
		totalAmount := amountIn.Add(decimal.NewFromUint64(constant.GasMODE[1]))
		remin := decimal.NewFromUint64(balance).Sub(totalAmount)
		if remin.LessThan(decimal.NewFromInt(0)) {
			return nil, xcode.SolBalanceNotEnough
		}
	}

	//step 2 : create ata account
	if market.InMint == solana.MustPublicKeyFromBase58(constant.Wsol) ||
		market.OutMint == solana.MustPublicKeyFromBase58(constant.Wsol) {
		tokenMint := market.OutMint
		programId := market.OutTokenProgram
		if market.SwapType == int32(trade.SwapType_Sell) {
			tokenMint = market.InMint
			programId = market.InTokenProgram
		}
		ataInst, err := amm.CreateAtaIdempotent(market.UserWalletAddress, market.UserWalletAddress, tokenMint, programId)
		if err != nil {
			return nil, fmt.Errorf("create ATA instruction err is :%v", err)
		}
		instruction = append(instruction, ataInst)
	} else {
		outTokenMint := market.OutMint
		outProgramId := market.OutTokenProgram
		inTokenMint := market.InMint
		inProgramId := market.InTokenProgram

		inAtaInst, err := amm.CreateAtaIdempotent(market.UserWalletAddress, market.UserWalletAddress, outTokenMint, outProgramId)
		if err != nil {
			return nil, fmt.Errorf("create ATA instruction err is :%v", err)
		}
		instruction = append(instruction, inAtaInst)

		outAtaInst, err := amm.CreateAtaIdempotent(market.UserWalletAddress, market.UserWalletAddress, inTokenMint, inProgramId)
		if err != nil {
			return nil, fmt.Errorf("create ATA instruction err is :%v", err)
		}
		instruction = append(instruction, outAtaInst)

	}

	if market.SwapType == int32(trade.SwapType_Buy) {
		inst, err := amm.createBuyInstruction(market)
		if err != nil {
			return nil, fmt.Errorf("create buy instruction err is :%v", err)
		}
		instruction = append(instruction, inst)
	}

	//step 2: add swap instruction
	//swapInstruction := amm.createSwapInstruction(market)
	//return swapInstruction

	return instruction, nil
}

func (amm *PumpfunAmm) CreateAtaIdempotent(payer, walletAddress, mint, programID aSDK.PublicKey) (solana.Instruction, error) {
	mintOwner := "<unknown>"
	acct, err := amm.client.GetAccountInfoWithOpts(context.TODO(), mint, &rpc.GetAccountInfoOpts{
		Encoding:   aSDK.EncodingBase64,
		Commitment: rpc.CommitmentProcessed,
	})
	if err != nil {
		amm.Infof("ATA debug: failed to fetch mint account owner for mint=%s err=%v", mint.String(), err)
	} else if acct != nil && acct.Value != nil {
		mintOwner = acct.Value.Owner.String()
	}
	amm.Infof(
		"ATA debug: payer=%s walletAddress=%s mint=%s requestedProgramID=%s actualMintOwner=%s",
		payer.String(),
		walletAddress.String(),
		mint.String(),
		programID.String(),
		mintOwner,
	)

	if programID == aSDK.TokenProgramID {
		return amm.CreateTokenAtaIdempotent(payer, walletAddress, mint)
	}

	if programID == aSDK.Token2022ProgramID {
		return amm.CreateToken2022AtaIdempotent(payer, walletAddress, mint)
	}

	return nil, fmt.Errorf("unsupported token program id: %s", programID.String())
}

func (amm *PumpfunAmm) CreateGasByGasFee(ctx context.Context, isAntiMev bool, walletAccount solana.PublicKey, cuLimit uint32, gasFeeInLamport uint64) ([]solana.Instruction, error) {

	priorityFee := decimal.NewFromUint64(gasFeeInLamport).Sub(decimal.NewFromUint64(constant.GasPerSignature))

	//1 lamport = 10 ^ 6 micro lamports
	gasMicroLamports := priorityFee.Mul(decimal.NewFromInt(1e6)).Div(decimal.NewFromUint64(uint64(cuLimit)))
	var insts []aSDK.Instruction
	if gasMicroLamports.IsPositive() {
		inst, err := computebudget.NewSetComputeUnitPriceInstruction(gasMicroLamports.BigInt().Uint64()).ValidateAndBuild()
		if err != nil {
			amm.Errorf("Error creating compute unit price instruction: %v", err)
			return nil, err
		}
		insts = append(insts, inst)

		inst, err = computebudget.NewSetComputeUnitLimitInstruction(uint32(cuLimit)).ValidateAndBuild()
		if err != nil {
			amm.Errorf("Error creating compute unit price instruction: %v", err)
			return nil, err
		}
		insts = append(insts, inst)

	}

	return insts, nil
}

func (amm *PumpfunAmm) CreateTokenAtaIdempotent(payer, walletAddress, mint aSDK.PublicKey) (aSDK.Instruction, error) {
	instruction, err := ata.NewCreateInstruction(payer, walletAddress, mint).ValidateAndBuild()
	if err != nil {
		return nil, err
	}

	instruction.TypeID = bin.TypeIDFromUint8(ata.Instruction_CreateIdempotent)

	return instruction, nil
}

func (amm *PumpfunAmm) CreateToken2022AtaIdempotent(payer, walletAddress, mint aSDK.PublicKey) (aSDK.Instruction, error) {
	amm.Infof("Creating Token2022 ATA: payer=%s, walletAddress=%s, mint=%s", payer.String(), walletAddress.String(), mint.String())
	associatedTokenAddress, _, err := token2022.FindAssociatedToken2022Address(walletAddress, mint)
	if err != nil {
		return nil, err
	}

	accounts := []*aSDK.AccountMeta{
		aSDK.Meta(payer).WRITE().SIGNER(),
		aSDK.Meta(associatedTokenAddress).WRITE(),
		aSDK.Meta(walletAddress),
		aSDK.Meta(mint),
		aSDK.Meta(aSDK.SystemProgramID),
		aSDK.Meta(aSDK.Token2022ProgramID),
	}

	// ATA program uses a single-byte discriminator for CreateIdempotent.
	return aSDK.NewInstruction(
		aSDK.SPLAssociatedTokenAccountProgramID,
		accounts,
		[]byte{byte(ata.Instruction_CreateIdempotent)},
	), nil
}
