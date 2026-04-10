package solana

import (
	"fmt"

	bin "github.com/gagliardetto/binary"
	aSDK "github.com/gagliardetto/solana-go"
	ata "github.com/gagliardetto/solana-go/programs/associated-token-account"
)

type AccountInstruction interface {
	CreateAtaIdempotent(payer, walletAddress, mint, programID aSDK.PublicKey) (aSDK.Instruction, error)
	CreateTokenAtaIdempotent(payer, walletAddress, mint aSDK.PublicKey) (aSDK.Instruction, error)
	CreateToken2022AtaIdempotent(payer, walletAddress, mint aSDK.PublicKey) (aSDK.Instruction, error)
}

func (tm *TxManager) CreateAtaIdempotent(payer, walletAddress, mint, programID aSDK.PublicKey) (aSDK.Instruction, error) {
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
	instruction := ata.NewCreateInstruction(payer, walletAddress, mint)
	instruction.AccountMetaSlice[5] = aSDK.Meta(aSDK.Token2022ProgramID)

	builtInstruction, err := instruction.ValidateAndBuild()
	if err != nil {
		return nil, err
	}

	builtInstruction.TypeID = bin.TypeIDFromUint8(ata.Instruction_CreateIdempotent)

	return builtInstruction, nil
}
