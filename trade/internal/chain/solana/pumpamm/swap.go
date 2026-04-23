package pumpamm

import (
	"encoding/binary"
	"fmt"
	"myDex/pkg/constant"
	"myDex/trade/internal/chain/solana/entity"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
)

type PoolAccount struct {
	PoolBump              uint8
	Index                 uint16
	Creator               solana.PublicKey
	BaseMint              solana.PublicKey
	QuoteMint             solana.PublicKey
	LpMint                solana.PublicKey
	PoolBaseTokenAccount  solana.PublicKey
	PoolQuoteTokenAccount solana.PublicKey
	LpSupply              uint64
	CoinCreator           solana.PublicKey
	IsMayhemMode          bool
}

type GlobalConfigAccount struct {
	// The admin pubkey
	Admin                  solana.PublicKey
	LpFeeBasisPoints       uint64
	ProtocolFeeBasisPoints uint64

	// Flags to disable certain functionality
	// bit 0 - Disable create pool
	// bit 1 - Disable deposit
	// bit 2 - Disable withdraw
	// bit 3 - Disable buy
	// bit 4 - Disable sell
	DisableFlags uint8

	// Addresses of the protocol fee recipients
	ProtocolFeeRecipients     [8]solana.PublicKey
	CoinCreatorFeeBasisPoints uint64

	// The admin authority for setting coin creators
	AdminSetCoinCreatorAuthority solana.PublicKey
	WhitelistPda                 solana.PublicKey
	ReservedFeeRecipient         solana.PublicKey
	MayhemModeEnabled            bool
	ReservedFeeRecipients        [7]solana.PublicKey
}

func (amm *PumpfunAmm) createBuyInstruction(market *entity.MarketTxExt) (solana.Instruction, error) {

	outMin := market.OutMint
	outTokenProgram := market.OutTokenProgram
	//step 1: 获取pool 账户
	pool, err := amm.getPoolAccountData(market.PairAddr)
	if err != nil {
		return nil, fmt.Errorf("createBuyInstruction: get pool account data err is : %v", err)
	}
	//step 2: 获取global config 账户
	globalAddr, err := amm.findFindGlobalConfigAddress()
	if err != nil {
		return nil, fmt.Errorf("createBuyInstruction: get global config address err is : %v", err)
	}
	globalConfig, err := amm.getGlobalConfigAccountInfo(globalAddr)
	if err != nil {
		return nil, fmt.Errorf("createBuyInstruction: get global config account data err is : %v", err)
	}
	//step 3: 获取protocol_fee_recipient_token account
	protocolFeeRecipientTokenAccountAddr, err := amm.
		findProtocolFeeRecipientTokenAccountAddress(globalConfig.ProtocolFeeRecipients[0],
			outTokenProgram,
			outMin)
	if err != nil {
		return nil, fmt.Errorf("createBuyInstruction: get protocol fee recipient token account address err is : %v", err)
	}
	//step 4: 获取event_authority
	eventAuthorityAddr, err := amm.findEventAuthorityAddress()
	if err != nil {
		return nil, err
	}
	amm.Infof("createBuyInstruction: eventAuthorityAddr=%s", eventAuthorityAddr.String())
	amm.Info("createBuyInstruction: pool=%s", pool.Creator.String())
	amm.Infof("createBuyInstruction: protocolFeeRecipientTokenAccountAddr=%s", protocolFeeRecipientTokenAccountAddr.String())
	return nil, nil

}

func (amm *PumpfunAmm) getPoolAccountData(pairAddr solana.PublicKey) (*PoolAccount, error) {
	result, err := amm.client.GetAccountInfoWithOpts(amm.ctx, pairAddr, &ag_rpc.GetAccountInfoOpts{
		Encoding:   solana.EncodingBase64,
		Commitment: ag_rpc.CommitmentConfirmed,
	})
	if err != nil {
		return nil, fmt.Errorf("createBuyInstruction: cquire pool account is err : %v", err)
	}
	binData := result.Value.Data.GetBinary()
	if len(binData) < 8 {
		return nil, fmt.Errorf("createBuyInstruction: pool account data length is less than 8")
	}

	decoder := bin.NewBinDecoder(binData[8:])

	var poolAccount PoolAccount
	if err := decoder.Decode(&poolAccount.PoolBump); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&poolAccount.Index); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&poolAccount.Creator); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&poolAccount.BaseMint); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&poolAccount.QuoteMint); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&poolAccount.LpMint); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&poolAccount.PoolBaseTokenAccount); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&poolAccount.PoolQuoteTokenAccount); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&poolAccount.LpSupply); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&poolAccount.CoinCreator); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&poolAccount.IsMayhemMode); err != nil {
		return nil, err
	}
	return &poolAccount, nil
}

func (amm *PumpfunAmm) findFindGlobalConfigAddress() (pda solana.PublicKey, err error) {
	var seeds [][]byte
	// const: global_config
	seeds = append(seeds, []byte("global_config"))

	pda, _, err = solana.FindProgramAddress(seeds, solana.MustPublicKeyFromBase58(constant.PumpAmmAddress))
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("findFindGlobalConfigAddress: can't find global Config account err is :%v", err)
	}
	return pda, nil
}

func (amm *PumpfunAmm) getGlobalConfigAccountInfo(global solana.PublicKey) (*GlobalConfigAccount, error) {
	result, err := amm.client.GetAccountInfoWithOpts(amm.ctx, global, &ag_rpc.GetAccountInfoOpts{
		Encoding:   solana.EncodingBase64,
		Commitment: ag_rpc.CommitmentConfirmed,
	})
	if err != nil {
		return nil, fmt.Errorf("createBuyInstruction: acquire global_config account is err : %v", err)
	}
	binData := result.Value.Data.GetBinary()
	if len(binData) < 8 {
		return nil, fmt.Errorf("createBuyInstruction: global_config account data length is less than 8")
	}

	data := binData[8:]
	offset := 8

	admin := readPubkey(data, &offset)
	lpFeeBasisPoints := binary.LittleEndian.Uint16(data[offset : offset+8])
	offset += 8
	protocolFeeBasisPoints := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8
	disableFlags := data[offset]
	offset += 1

	var feeRecipients [8]solana.PublicKey
	for i := 0; i < 8; i++ {
		feeRecipients[i] = readPubkey(data, &offset)
	}

	coinCreatorFeeBasisPoints := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8
	adminSetCoinCreatorAuthority := readPubkey(data, &offset)

	whitelistPda := readPubkey(data, &offset)

	reservedFeeRecipient := readPubkey(data, &offset)

	mayhemModeEnabled := data[offset]
	offset += 1

	var reservedFeeRecipients [7]solana.PublicKey
	for i := 0; i < 7; i++ {
		reservedFeeRecipients[i] = readPubkey(data, &offset)
	}

	return &GlobalConfigAccount{
		Admin:                        admin,
		LpFeeBasisPoints:             uint64(lpFeeBasisPoints),
		ProtocolFeeBasisPoints:       protocolFeeBasisPoints,
		DisableFlags:                 disableFlags,
		ProtocolFeeRecipients:        feeRecipients,
		CoinCreatorFeeBasisPoints:    coinCreatorFeeBasisPoints,
		AdminSetCoinCreatorAuthority: adminSetCoinCreatorAuthority,
		WhitelistPda:                 whitelistPda,
		ReservedFeeRecipient:         reservedFeeRecipient,
		MayhemModeEnabled:            mayhemModeEnabled == 1,
		ReservedFeeRecipients:        reservedFeeRecipients,
	}, nil
}

func readPubkey(data []byte, offset *int) solana.PublicKey {
	var pk solana.PublicKey
	copy(pk[:], data[*offset:*offset+32])
	*offset += 32
	return pk
}

func (amm *PumpfunAmm) findProtocolFeeRecipientTokenAccountAddress(protocolFeeRecipient solana.PublicKey, quoteTokenProgram solana.PublicKey, quoteMint solana.PublicKey) (pda solana.PublicKey, err error) {
	var seeds [][]byte
	// path: protocol_fee_recipient
	seeds = append(seeds, protocolFeeRecipient.Bytes())
	// path: quote_token_program
	seeds = append(seeds, quoteTokenProgram.Bytes())
	// path: quote_mint
	seeds = append(seeds, quoteMint.Bytes())

	programID := solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL") //["ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"]

	pda, _, err = solana.FindProgramAddress(seeds, programID)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("find ProtocolFeeRecipientTokenAccount err is: %v", err)
	}
	return pda, nil
}

func (amm *PumpfunAmm) findEventAuthorityAddress() (pda solana.PublicKey, err error) {
	var seeds [][]byte
	// const: __event_authority
	seeds = append(seeds, []byte("__event_authority"))

	pda, _, err = solana.FindProgramAddress(seeds, solana.MustPublicKeyFromBase58(constant.PumpAmmAddress))
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("find EventAuthorityAddress err is: %v", err)
	}
	return pda, nil
}
