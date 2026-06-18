package solana

import (
	"context"
	"crypto/ed25519"
	"myDex/pkg/constant"
	"myDex/trade/internal/chain/solana/entity"
	"myDex/trade/internal/chain/solana/pumpamm"
	"sync"

	// "dex/pkg/raydium/clmm/idl/generated/amm_v3"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mr-tron/base58"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"

	aSDK "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logc"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
	"gorm.io/gorm"
)

type GasType int32

const (
	GasType_GasTypeSpeedInvalid GasType = 0
	GasType_NormalSpeed         GasType = 1
	GasType_FastSpeed           GasType = 2
	GasType_SuperFastSpeed      GasType = 3
)

type CreateMarketTx struct {
	UserId            uint64
	ChainId           uint64
	UserWalletId      uint32
	UserWalletAccount solana.PublicKey
	AmountIn          string
	IsAntiMev         bool
	Slippage          uint32
	IsAutoSlippage    bool
	GasType           int32
	TradePoolName     string
	InDecimal         uint8
	OutDecimal        uint8
	InMint            solana.PublicKey
	OutMint           solana.PublicKey
	PairAddr          string
	Price             string
	UsePriceLimit     bool
	InTokenProgram    solana.PublicKey
	OutTokenProgram   solana.PublicKey
}

type TxManager struct {
	Client       *ag_rpc.Client
	MainClient   *ag_rpc.Client
	JitoClient   *ag_rpc.Client
	DB           *gorm.DB
	rentFee      uint64
	context      context.Context
	SimulateOnly bool
	logx.Logger
	pumpFunAmm   *pumpamm.PumpfunAmm
	jitoTipFloor *JitoTipFloor
	RWLock       sync.RWMutex
}

func NewTxManager(db *gorm.DB, rpcEndpoint string, mainRpcEndpoint string, simulateOnly bool, jitoEndPoint string) *TxManager {
	var jitoClient *ag_rpc.Client
	if len(jitoEndPoint) > 0 {
		jitoClient = ag_rpc.New(jitoEndPoint)
	}

	tm := &TxManager{
		DB:           db,
		Client:       ag_rpc.New(rpcEndpoint),
		MainClient:   ag_rpc.New(mainRpcEndpoint),
		JitoClient:   jitoClient,
		SimulateOnly: simulateOnly,
		context:      context.Background(),
		Logger:       logx.WithContext(context.Background()).WithFields(logx.LogField{Key: "service", Value: "txManage"}),
		pumpFunAmm:   pumpamm.NewPumpfunAmm(rpcEndpoint),
	}

	if len(jitoEndPoint) > 0 {
		tm.jitoTipFloor = &JitoTipFloor{
			Time:                        time.Now(),
			LandedTips25ThPercentile:    0.000001,
			LandedTips50ThPercentile:    0.00001,
			LandedTips75ThPercentile:    0.00004,
			LandedTips95ThPercentile:    0.006,
			LandedTips99ThPercentile:    0.018,
			EmaLandedTips50ThPercentile: 0.000014,
		}
		threading.GoSafe(func() {
			tm.CheckJitoFloorFee()
		})
	}

	threading.GoSafe(func() {
		tm.CheckRentFee()
	})

	return tm
}

func convertMarketTx(in *entity.MarketTx) (*entity.MarketTxExt, error) {

	userWalletAccount, err := aSDK.PublicKeyFromBase58(in.UserWalletAddress)
	if err != nil {
		fmt.Printf("Error parsing UserWalletAddress '%s': %v\n", in.UserWalletAddress, err)
		return nil, err
	}
	inMint, err := aSDK.PublicKeyFromBase58(in.InTokenCa)
	if err != nil {
		fmt.Printf("Error parsing InTokenCa '%s': %v\n", in.InTokenCa, err)
		return nil, err
	}
	outMint, err := aSDK.PublicKeyFromBase58(in.OutTokenCa)
	if err != nil {
		fmt.Printf("Error parsing OutTokenCa '%s': %v\n", in.OutTokenCa, err)
		return nil, err
	}

	inTokenProgram, err := aSDK.PublicKeyFromBase58(in.InTokenProgram)
	if err != nil {
		fmt.Printf("Error parsing InTokenProgram '%s': %v\n", in.InTokenProgram, err)
		return nil, err
	}

	outTokenProgram, err := aSDK.PublicKeyFromBase58(in.OutTokenProgram)
	if err != nil {
		fmt.Printf("Error parsing OutTokenProgram '%s': %v\n", in.OutTokenProgram, err)
		return nil, err
	}

	return &entity.MarketTxExt{
		UserId:            in.UserId,
		ChainId:           in.ChainId,
		UserWalletId:      in.UserWalletId,
		UserWalletAddress: userWalletAccount,
		AmountIn:          in.AmountIn,
		IsAntiMev:         in.IsAntiMev,
		Slippage:          in.Slippage,
		IsAutoSlippage:    in.IsAutoSlippage,
		SwapType:          in.SwapType,
		TradePoolName:     in.TradePoolName,
		InDecimal:         in.InDecimal,
		OutDecimal:        in.OutDecimal,
		InMint:            inMint,
		OutMint:           outMint,
		PairAddr:          solana.MustPublicKeyFromBase58(in.PairAddr),
		Price:             in.Price,
		InTokenProgram:    inTokenProgram,
		OutTokenProgram:   outTokenProgram,
	}, nil
}

// 模拟执行交易。检查交易是否成功，不会发送上链
func (tm *TxManager) simulate(ctx context.Context, tx *aSDK.Transaction) error {
	tm.Infof("simulate start")
	simOut, err := tm.MainClient.SimulateTransactionWithOpts(ctx, tx, &ag_rpc.SimulateTransactionOpts{
		Commitment: ag_rpc.CommitmentProcessed,
	})
	if err != nil {
		logc.Error(ctx, err)
		return err
	}
	if nil != simOut && nil != simOut.Value && simOut.Value.Err != nil {
		logs := strings.Join(simOut.Value.Logs, " ")
		logc.Infof(ctx, "simOut failed , logs %s , err:%v", logs, simOut.Value.Err)
		return errors.New(logs)
	}
	tm.Infof("simulate done")
	return nil
}

// BuildUnsignedTransaction builds an unsigned transaction for third-party wallet signing
func (tm *TxManager) BuildUnsignedTransaction(ctx context.Context, createMarketTx *entity.MarketTx) (string, error) {

	tm.Infof("Building unsigned transaction for third-party wallet signing")

	in, err := convertMarketTx(createMarketTx)
	if err != nil {
		return "", err
	}

	//step 1: get instructions
	var instructions []aSDK.Instruction
	switch in.TradePoolName {
	case constant.PumpFunName:
		instructions, err = tm.CreateMarketOrderPumpfun(ctx, in)
		if err != nil {
			return "", err
		}
	case constant.PumpFunAmmName:
		instructions, err = tm.pumpFunAmm.CreateMarketOrderPumpfunAmm(in)
		if err != nil {
			return "", err
		}
	// case constants.RaydiumV4, constants.RaydiumConcentratedLiquidity, constants.RaydiumCPMM:
	// 	instructions, err = tm.CreateMarketOrderDex(ctx, in)
	// 	if err != nil {
	// 		return "", err
	// 	}
	default:
		return "", fmt.Errorf("TradePoolName:%s not support", in.TradePoolName)
	}
	tm.Infof("BuildUnsignedTransaction instructions ready, count=%d tradePool=%s", len(instructions), in.TradePoolName)

	// 设置rpc调用最多15s超时时间
	//step 2: get latest blockhash
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := tm.Client.GetLatestBlockhash(timeoutCtx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("failed to get latest blockhash: %w", err)
	}
	tm.Infof("GetLatestBlockhash done, blockhash=%s", resp.Value.Blockhash.String())

	// 创建未签名交易
	//step 3: create unsigned transaction
	feePayer, err := aSDK.PublicKeyFromBase58(createMarketTx.UserWalletAddress)
	if err != nil {
		return "", err
	}
	tm.Infof("NewTransaction start, feePayer=%s", feePayer.String())
	tx, err := aSDK.NewTransaction(instructions, resp.Value.Blockhash, aSDK.TransactionPayer(feePayer))
	if err != nil {
		return "", err
	}

	// 初始化一个空的签名数组，长度为交易消息头中指定的需要签名的账户数量
	// step 3.1: Initialize empty signatures for the transaction
	numSigners := int(tx.Message.Header.NumRequiredSignatures)
	tx.Signatures = make([]aSDK.Signature, numSigners)

	// 序列化交易对象，并用base64编码返回给调用方，供第三方钱包签名使用
	// step 3.2: Serialize the complete transaction (with empty signatures)
	txData, err := tx.MarshalBinary()
	if err != nil {
		logx.WithContext(ctx).Errorf("Failed to serialize transaction: %v", err)
		return "", err
	}
	tm.Infof("BuildUnsignedTransaction done, signers=%d serializedBytes=%d", numSigners, len(txData))

	// Return the serialized transaction as base64
	return base64.StdEncoding.EncodeToString(txData), nil
}

// SignTransaction signs an unsigned transaction (base64) using the service's private key and returns the signature
func (tm *TxManager) SignTransaction(ctx context.Context, tx string) (aSDK.Transaction, string, error) {
	tm.Infof("SignTransaction decode start, txBase64Len=%d", len(tx))
	//step 1: decode the base64 transaction
	txData, err := base64.StdEncoding.DecodeString(tx)
	if err != nil {
		return aSDK.Transaction{}, "", fmt.Errorf("failed to decode transaction: %v", err)
	}
	tm.Infof("SignTransaction decode done, txBytes=%d", len(txData))

	//step 2: unmarshal the transaction
	var unsignedTx aSDK.Transaction
	//create bin decoder with the transaction data and decode into the unsignedTx struct
	decoder := bin.NewBinDecoder(txData)
	if err := decoder.Decode(&unsignedTx); err != nil {
		return aSDK.Transaction{}, "", fmt.Errorf("failed to unmarshal transaction: %v", err)
	}
	tm.Infof("SignTransaction unmarshal done, requiredSigners=%d", unsignedTx.Message.Header.NumRequiredSignatures)

	//step 3: sign the transaction using the service's private key
	privateKey := os.Getenv("private_key")
	if privateKey == "" {
		return aSDK.Transaction{}, "", fmt.Errorf("private key not set in environment variable")
	}
	// Decode base58 private key
	privateKeyBytes, err := base58.Decode(privateKey)
	if err != nil {
		return aSDK.Transaction{}, "", fmt.Errorf("failed to decode base58 private key: %v", err)
	}
	// judge the length of private key bytes, ed25519 private key should be 64 bytes
	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		return aSDK.Transaction{}, "", fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.PrivateKeySize, len(privateKeyBytes))
	}
	ed25519PrivateKey := ed25519.PrivateKey(privateKeyBytes)

	// Sign the transaction message
	messageContent, err := unsignedTx.Message.MarshalBinary()
	if err != nil {
		logc.Error(ctx, err)
		return aSDK.Transaction{}, "", fmt.Errorf("failed to marshal transaction message: %v", err)
	}
	tm.Infof("SignTransaction message marshaled, messageBytes=%d", len(messageContent))
	signature := ed25519.Sign(ed25519PrivateKey, messageContent)

	// Set signatures for all required signers (assuming single signer for now)
	numSigners := int(unsignedTx.Message.Header.NumRequiredSignatures)
	unsignedTx.Signatures = make([]aSDK.Signature, numSigners)
	for i := 0; i < numSigners; i++ {
		copy(unsignedTx.Signatures[i][:], signature)
	}

	// Return the signature of the first signer as a string
	if tm.SimulateOnly {
		timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		tm.Infof("SignTransaction simulate enabled")
		err = tm.simulate(timeoutCtx, &unsignedTx)
		if err != nil {
			return aSDK.Transaction{}, "", err
		}
	}
	tm.Infof("SignTransaction done")
	return unsignedTx, unsignedTx.Signatures[0].String(), nil

}

func (tm *TxManager) SendWithSignTransaction(ctx context.Context, transcation aSDK.Transaction) (string, error) {
	tm.Infof("SendWithSignTransaction start")

	tx := &transcation
	if len(tx.Signatures) == 0 {
		return "", fmt.Errorf("transaction has no signature")
	}

	if tm.SimulateOnly {
		err := tm.simulate(ctx, tx)
		if err != nil {
			return "", err
		}

		return tx.Signatures[0].String(), nil
	}

	if tm.HasJitoTip(tx) {
		sig, err := tm.SendViaJitoRetry(ctx, tx)
		if nil != err {
			logc.Infof(ctx, "SendWithSignTransaction via jito failed:%s", err.Error())
			return "", err
		}
		return sig, nil
	}

	sig, err := tm.Client.SendTransactionWithOpts(ctx, &transcation, ag_rpc.TransactionOpts{
		SkipPreflight:       false,
		PreflightCommitment: ag_rpc.CommitmentConfirmed,
	})
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %v", err)
	}
	if sig.IsZero() {
		return "", fmt.Errorf("send transaction returned empty signature")
	}
	tm.Infof("SendWithSignTransaction done, signature=%s", sig.String())
	return sig.String(), nil
}
