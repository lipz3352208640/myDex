package entity

import "github.com/gagliardetto/solana-go"

type SellInstruction struct {
	Amount                 uint64
	MinSolOutput           uint64
	Global                 solana.PublicKey
	FeeRecipient           solana.PublicKey
	Mint                   solana.PublicKey
	BondingCurve           solana.PublicKey
	AssociatedBondingCurve solana.PublicKey
	AssociatedUser         solana.PublicKey
	User                   solana.PublicKey
	SystemProgram          solana.PublicKey
	CreatorVault           solana.PublicKey
	TokenProgram           solana.PublicKey
	EventAuthority         solana.PublicKey
	Program                solana.PublicKey
	FeeConfig              solana.PublicKey
	FeeProgram             solana.PublicKey
}

type BuyInstruction struct {
	SpendableSolIn          uint64
	MinTokensOut            uint64
	Global                  solana.PublicKey
	FeeRecipient            solana.PublicKey
	Mint                    solana.PublicKey
	BondingCurve            solana.PublicKey
	AssociatedBondingCurve  solana.PublicKey
	AssociatedUser          solana.PublicKey
	User                    solana.PublicKey
	SystemProgram           solana.PublicKey
	TokenProgram            solana.PublicKey
	CreatorVault            solana.PublicKey
	EventAuthority          solana.PublicKey
	Program                 solana.PublicKey
	GlobalVolumeAccumulator solana.PublicKey
	UserVolumeAccumulator   solana.PublicKey
	FeeConfig               solana.PublicKey
	FeeProgram              solana.PublicKey
}
