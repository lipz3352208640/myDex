package solana

import (
	"context"
	"fmt"
	token2022 "myDex/pkg/token2022"

	bin "github.com/gagliardetto/binary"
	aSDK "github.com/gagliardetto/solana-go"
	ata "github.com/gagliardetto/solana-go/programs/associated-token-account"
	"github.com/gagliardetto/solana-go/rpc"
)

type AccountInstruction interface {
	CreateAtaIdempotent(payer, walletAddress, mint, programID aSDK.PublicKey) (aSDK.Instruction, error)
	CreateTokenAtaIdempotent(payer, walletAddress, mint aSDK.PublicKey) (aSDK.Instruction, error)
	CreateToken2022AtaIdempotent(payer, walletAddress, mint aSDK.PublicKey) (aSDK.Instruction, error)
}

func (tm *TxManager) CreateAtaIdempotent(payer, walletAddress, mint, programID aSDK.PublicKey) (aSDK.Instruction, error) {
	mintOwner := "<unknown>"
	acct, err := tm.Client.GetAccountInfoWithOpts(context.TODO(), mint, &rpc.GetAccountInfoOpts{
		Encoding:   aSDK.EncodingBase64,
		Commitment: rpc.CommitmentProcessed,
	})
	if err != nil {
		tm.Infof("ATA debug: failed to fetch mint account owner for mint=%s err=%v", mint.String(), err)
	} else if acct != nil && acct.Value != nil {
		mintOwner = acct.Value.Owner.String()
	}
	tm.Infof(
		"ATA debug: payer=%s walletAddress=%s mint=%s requestedProgramID=%s actualMintOwner=%s",
		payer.String(),
		walletAddress.String(),
		mint.String(),
		programID.String(),
		mintOwner,
	)

	if programID == aSDK.TokenProgramID {
		return tm.CreateTokenAtaIdempotent(payer, walletAddress, mint)
	}

	if programID == aSDK.Token2022ProgramID {
		return tm.CreateToken2022AtaIdempotent(payer, walletAddress, mint)
	}

	return nil, fmt.Errorf("unsupported token program id: %s", programID.String())
}

func (tm *TxManager) CreateTokenAtaIdempotent(payer, walletAddress, mint aSDK.PublicKey) (aSDK.Instruction, error) {
	instruction, err := ata.NewCreateInstruction(payer, walletAddress, mint).ValidateAndBuild()
	if err != nil {
		return nil, err
	}

	instruction.TypeID = bin.TypeIDFromUint8(ata.Instruction_CreateIdempotent)

	return instruction, nil
}

func (tm *TxManager) CreateToken2022AtaIdempotent(payer, walletAddress, mint aSDK.PublicKey) (aSDK.Instruction, error) {
	tm.Infof("Creating Token2022 ATA: payer=%s, walletAddress=%s, mint=%s", payer.String(), walletAddress.String(), mint.String())
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
