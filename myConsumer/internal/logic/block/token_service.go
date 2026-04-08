package block

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"myDex/model/solmodel"
	"myDex/myConsumer/internal/logic/entity"
	"myDex/myConsumer/internal/svc"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/metaplex/token_metadata"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
)

type TokenService interface {
	SaveToken(trades *entity.TradeWithPair) (tokenDB *solmodel.Token, err error)
}

type TokenServiceImpl struct {
	ctx    context.Context
	cancel func(error error)
	logx.Logger
	sc     *svc.ServiceContext
	c      *client.Client
	rpcUrl string
}

func (t *TokenServiceImpl) SaveToken(trade *entity.TradeWithPair) (tokenDB *solmodel.Token, err error) {

	//首先判断token是否存在
	chainId := trade.ChainId
	chainInt, _ := strconv.ParseInt(chainId, 10, 64)
	tokenDB, err = t.sc.TokenModel.FindOneByChainIdAddress(t.ctx, chainInt, trade.PairInfo.TokenAddr)

	opts := &jsonrpc.RPCClientOpts{
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	rpcClient := jsonrpc.NewClientWithOpts(t.rpcUrl, opts)
	if err == nil && tokenDB != nil {
		hasChange := false
		if tokenDB.Slot == 0 {
			tokenDB.Slot = trade.Slot
			hasChange = true
		}

		//总发行量为0
		if tokenDB.TotalSupply == 0 {
			totalSupply, err := GetTokenTotalSupply(t.c, t.ctx, tokenDB.Address)
			t.Infof("SaveToken:GetTokenMeta update token totalSupply: token addr: %v, totalSupply: %v", tokenDB.Address, totalSupply)
			if err == nil {
				tokenDB.TotalSupply = totalSupply.InexactFloat64()
				hasChange = true
			} else {
				t.Errorf("SaveToken:GetTokenTotalSupply update err:%v, address: %v", err, tokenDB.Address)
			}
		}

		//合约不存在
		if len(tokenDB.Program) == 0 {
			program, _ := GetTokenProgram(t.c, t.ctx, tokenDB.Address)
			switch program {
			case common.TokenProgramID:
				tokenDB.Program = common.TokenProgramID.String()
				hasChange = true
			case common.Token2022ProgramID:
				tokenDB.Program = common.Token2022ProgramID.String()
				hasChange = true
			default:

			}
		}

		if tokenDB.Symbol == "" || tokenDB.Name == "" {
			switch tokenDB.Program {
			case common.TokenProgramID.String():
				tokenInfo, error := GetTokenInfo(t.c, t.ctx, tokenDB.Address)

				if error != nil {
					t.Errorf("SaveToken:GetTokenInfo update err: %v, address: %v", err, tokenDB.Address)
				}
				if tokenInfo != nil {
					tokenDB.Symbol = tokenInfo.MetaData.Data.Symbol
					tokenDB.Name = tokenInfo.MetaData.Data.Name
					tokenDB.TwitterUsername = tokenInfo.Uri.Twitter
					tokenDB.Website = tokenInfo.Uri.Website
					tokenDB.Telegram = tokenInfo.Uri.Telegram
					tokenDB.Icon = tokenInfo.Uri.Image
					tokenDB.Description = tokenInfo.Uri.Description

					if len(tokenInfo.Uri.Symbol) > 0 {
						tokenDB.Symbol = tokenInfo.Uri.Symbol
					}
					if len(tokenInfo.Uri.Name) > 0 {
						tokenDB.Name = tokenInfo.Uri.Name
					}

					hasChange = true
				}
			case common.Token2022ProgramID.String():
				_, tokenInfo, err := GetToken2022Info(ag_rpc.NewWithCustomRPCClient(rpcClient), t.ctx, solana.MustPublicKeyFromBase58(tokenDB.Address))
				if err != nil {
					t.Errorf("SaveToken:GetToken2022Info err: %v, token address: %v", err, tokenDB.Address)
				}

				if tokenInfo != nil {

					tokenDB.Symbol = tokenInfo.Symbol
					tokenDB.Name = tokenInfo.Name

					tokenDB.TwitterUsername = tokenInfo.Uri.Twitter
					tokenDB.Website = tokenInfo.Uri.Website
					tokenDB.Telegram = tokenInfo.Uri.Telegram
					tokenDB.Icon = tokenInfo.Uri.Image

					tokenDB.Description = tokenInfo.Uri.Description

					if len(tokenInfo.Uri.Name) > 0 {
						tokenDB.Name = tokenInfo.Uri.Name
					}

					if len(tokenInfo.Uri.Symbol) > 0 {
						tokenDB.Symbol = tokenInfo.Uri.Symbol
					}

					hasChange = true

				}
			default:

			}
		}

		if hasChange {

			err := t.sc.TokenModel.Update(t.ctx, tokenDB)
			if err != nil {
				return tokenDB, err
			}
		}
		return tokenDB, nil
	}

	if errors.Is(err, solmodel.ErrNotFound) {

		tokenDB = &solmodel.Token{
			ChainId:  chainInt,
			Address:  trade.PairInfo.TokenAddr,
			Decimals: int64(trade.PairInfo.TokenDecimal),
			Slot:     trade.Slot,
		}

		totalSupply, err := GetTokenTotalSupply(t.c, t.ctx, tokenDB.Address)
		if err == nil {
			tokenDB.TotalSupply = totalSupply.InexactFloat64()
		} else {
			t.Errorf("SaveToken:GetTokenTotalSupply insert err:%v, address: %v", err, tokenDB.Address)
		}

		program, _ := GetTokenProgram(t.c, t.ctx, tokenDB.Address)
		switch program {
		case common.Token2022ProgramID:
			tokenDB.Program = common.Token2022ProgramID.String()

			_, tokenInfo, err := GetToken2022Info(ag_rpc.NewWithCustomRPCClient(rpcClient), t.ctx, solana.MustPublicKeyFromBase58(tokenDB.Address))
			if err != nil {
				t.Errorf("SaveToken:GetToken2022Info err: %v, token address: %v", err, tokenDB.Address)
			}

			if tokenInfo != nil {

				tokenDB.Symbol = tokenInfo.Symbol
				tokenDB.Name = tokenInfo.Name

				tokenDB.TwitterUsername = tokenInfo.Uri.Twitter
				tokenDB.Website = tokenInfo.Uri.Website
				tokenDB.Telegram = tokenInfo.Uri.Telegram
				tokenDB.Icon = tokenInfo.Uri.Image
				tokenDB.Description = tokenInfo.Uri.Description

				if len(tokenInfo.Uri.Name) > 0 {
					tokenDB.Name = tokenInfo.Uri.Name
				}

				if len(tokenInfo.Uri.Symbol) > 0 {
					tokenDB.Symbol = tokenInfo.Uri.Symbol
				}
			}
		default:
			tokenDB.Program = common.TokenProgramID.String()

			tokenInfo, err := GetTokenInfo(t.c, t.ctx, tokenDB.Address)
			if err != nil {
				t.Errorf("SaveToken:GetTokenInfo err: %v, address: %v", err, tokenDB.Address)
			}
			if tokenInfo != nil {
				tokenDB.Symbol = tokenInfo.MetaData.Data.Symbol
				tokenDB.Name = tokenInfo.MetaData.Data.Name
				tokenDB.TwitterUsername = tokenInfo.Uri.Twitter
				tokenDB.Website = tokenInfo.Uri.Website
				tokenDB.Telegram = tokenInfo.Uri.Telegram
				tokenDB.Icon = tokenInfo.Uri.Image
				tokenDB.Description = tokenInfo.Uri.Description

				if len(tokenInfo.Uri.Symbol) > 0 {
					tokenDB.Symbol = tokenInfo.Uri.Symbol
				}
				if len(tokenInfo.Uri.Name) > 0 {
					tokenDB.Name = tokenInfo.Uri.Name
				}
				// tokenDB.SetSolTokenDefaultCa()
				// tokenDB.IsCanAddToken = int64(tokenInfo.IsCanAddToken)
			}
		}

		err = t.sc.TokenModel.Insert(t.ctx, tokenDB)
		if err != nil {
			if strings.Contains(err.Error(), "Duplicate entry") {
				// db already exists
				tokenDB, err = t.sc.TokenModel.FindOneByChainIdAddress(t.ctx, chainInt, trade.PairInfo.TokenAddr)
				if err != nil {
					return nil, err
				}
				return tokenDB, nil
			}
			return nil, err
		}

		t.Infof("SaveToken:GetTokenInfo insert success address: %v, token: %#v, program: %v", tokenDB.Address, tokenDB, program)

		return tokenDB, nil
	}

	return nil, fmt.Errorf("SaveToken err:%w", err)
}

// 获取代币总发行量
func GetTokenTotalSupply(c *client.Client, ctx context.Context, address string) (decimal.Decimal, error) {
	supplyModel, err := c.GetTokenSupplyWithConfig(ctx, address, client.GetTokenSupplyConfig{
		Commitment: rpc.CommitmentFinalized,
	})
	if err != nil {
		err = fmt.Errorf("GetTokenTotalSupply token err:%v,token address: %v", err, address)
		return decimal.Zero, err
	}
	totalSupply := decimal.NewFromInt(int64(supplyModel.Amount)).Div(decimal.New(1, int32(supplyModel.Decimals)))
	return totalSupply, nil
}

// 获取token所在合约
func GetTokenProgram(c *client.Client, ctx context.Context, address string) (program common.PublicKey, err error) {
	accountInfo, err := c.GetAccountInfoWithConfig(ctx, address, client.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})

	if err != nil {
		err = fmt.Errorf("GetTokenMintInfo token err:%v, token address: %v", err, address)
		return
	}

	switch accountInfo.Owner {
	case common.Token2022ProgramID:
		return common.Token2022ProgramID, nil
	case common.TokenProgramID:
		return common.TokenProgramID, nil
	}
	return common.PublicKey{}, errors.New("not support")
}

// 获取token所在合约
func GetTokenInfo(c *client.Client, ctx context.Context, address string) (tokenInfo *entity.TokenInfo, err error) {

	tokenInfo = &entity.TokenInfo{}

	//获取token 账户信息
	accountInfo, err := c.GetAccountInfoWithConfig(ctx, address, client.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})

	if err != nil {
		err = fmt.Errorf("GetAccountInfoWithConfig token err:%v, token address: %v", err, address)
	}

	if len(accountInfo.Data) == 0 {
		err = fmt.Errorf("GetTokenInfo:GetAccountInfoWithConfig token data is nil, err:%v, token address: %#v", err, address)
	}

	if accountInfo.Owner == common.Token2022ProgramID {
		tokenInfo.ProgramId = common.Token2022ProgramID.String()
	}
	if accountInfo.Owner == common.TokenProgramID {
		tokenInfo.ProgramId = common.TokenProgramID.String()
	}

	//获取mint账户
	mintAccount, err := token.MintAccountFromData(accountInfo.Data[:82])
	if err != nil {
		err = fmt.Errorf("GetTokenInfo:MintAccountFromData err:%v, token address: %v", err, address)
	}
	tokenInfo.MintCount = mintAccount

	//获取token的元数据账户信息
	metaAddress, err := token_metadata.GetTokenMetaPubkey(common.PublicKeyFromString(address))
	if err != nil {
		err = fmt.Errorf("GetTokenInfo:GetTokenMetaPubkey err:%v, token address: %v", err, address)
	}
	resp, err := c.GetAccountInfoWithConfig(ctx, metaAddress.String(), client.GetAccountInfoConfig{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		err = fmt.Errorf("GetTokenInfo:GetAccountInfoWithConfig by MetaAddress err:%v, token address: %v", err, metaAddress)
	}

	if len(resp.Data) <= 0 {
		return
	}

	metaData, err := token_metadata.MetadataDeserialize(resp.Data)
	if err != nil {
		err = fmt.Errorf("deserialize metaAccount data err:%w", err)
		return
	}

	tokenInfo.MetaData = metaData

	//获取链下json 链下json保存meta完整信息
	if len(metaData.Data.Uri) > 0 {
		publicGateway := "https://ipfs.io/ipfs/"
		if !isURLAccessible(metaData.Data.Uri) {
			metaData.Data.Uri = replaceWithPublicGateway(metaData.Data.Uri, publicGateway)
		}

		ctx, cancelFunc := context.WithTimeout(context.Background(), 3000*time.Millisecond)
		defer cancelFunc()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, metaData.Data.Uri, nil)
		if err != nil {
			err = fmt.Errorf("http.NewRequest err:%w", err)
			return tokenInfo, err
		}

		// 执行请求
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			// skip error
			return tokenInfo, err
		}
		defer func() {
			_ = response.Body.Close()
		}()

		res, err := io.ReadAll(response.Body)

		if err != nil {
			return tokenInfo, err
		}
		// 检查 Content-Type
		contentType := response.Header.Get("Content-Type")
		if strings.Contains(string(res), "Account has been disabled.") {
			return tokenInfo, nil
		}
		if strings.HasPrefix(contentType, "application/json") {
			tokenUriData, err := Byte2Struct[entity.TokenUriData](res)
			if err != nil {
				return tokenInfo, nil
			}

			fmt.Println("meta json 内容", string(res))

			if len(tokenUriData.Website) == 0 {
				tokenUriData.Website = tokenUriData.Website
			}
			if len(tokenUriData.Telegram) == 0 {
				tokenUriData.Telegram = tokenUriData.Telegram
			}
			if len(tokenUriData.Twitter) == 0 {
				tokenUriData.Twitter = tokenUriData.Twitter
			}
			tokenInfo.Uri = tokenUriData
		} else if strings.HasPrefix(contentType, "image/") {
			// skip error
			tokenInfo.Uri.Image = metaData.Data.Uri
		} else {
			// default
			tokenUriData, err := Byte2Struct[entity.TokenUriData](res)

			if err != nil {
				// err = fmt.Errorf("GetTokenInfo error: %v, url: %v, token address: %v", err, metaData.Data.Uri, address)
				return tokenInfo, nil
			}

			if len(tokenUriData.Website) == 0 {
				tokenUriData.Website = tokenUriData.Website
			}
			if len(tokenUriData.Telegram) == 0 {
				tokenUriData.Telegram = tokenUriData.Telegram
			}
			if len(tokenUriData.Twitter) == 0 {
				tokenUriData.Twitter = tokenUriData.Twitter
			}

			tokenInfo.Uri = tokenUriData

		}
	}
	return tokenInfo, err
}

func GetToken2022Info(c *ag_rpc.Client, ctx context.Context, address solana.PublicKey) (token2022Info *entity.Info, tokenInfo *entity.TokenInfo, err error) {
	resp, err := c.GetAccountInfoWithOpts(ctx, address, &ag_rpc.GetAccountInfoOpts{
		Encoding:   solana.EncodingJSONParsed,
		Commitment: ag_rpc.CommitmentConfirmed,
	})
	if err != nil || resp == nil {
		err = fmt.Errorf("GetToken2022Info:GetAccountInfoWithOpts token err:%v, token address: %v", err, address)
		return
	}

	if len(resp.Value.Data.GetRawJSON()) == 0 {
		err = fmt.Errorf("GetToken2022Info:GetAccountInfoWithOpts token data is nil, err:%v, token address: %#v", err, address)
		return nil, nil, err
	}

	fmt.Println("token 2022  json ", string(resp.Value.Data.GetRawJSON()))

	mintResponse, err := Byte2Struct[*entity.MintResponse](resp.Value.Data.GetRawJSON())
	if err != nil {
		err = fmt.Errorf("GetToken2022Info:Byte2Struct err:%v, token address: %v", err, address)
		return
	}

	tokenInfo = &entity.TokenInfo{}

	for _, extension := range mintResponse.Parsed.Info.Extensions {
		if extension.Extension == "tokenMetadata" {
			tokenInfo.Name = extension.State.Name
			tokenInfo.Symbol = extension.State.Symbol

			if len(extension.State.Uri) > 0 {
				publicGateway := "https://ipfs.io/ipfs/"
				if !isURLAccessible(extension.State.Uri) {
					extension.State.Uri = replaceWithPublicGateway(extension.State.Uri, publicGateway)
				}

				ctx, cancelFunc := context.WithTimeout(context.Background(), 3000*time.Millisecond)
				defer cancelFunc()
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, extension.State.Uri, nil)
				if err != nil {
					err = fmt.Errorf("http.NewRequest err:%w", err)
					return token2022Info, tokenInfo, err
				}

				// 执行请求
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					// skip error
					return token2022Info, tokenInfo, err
				}
				defer func() {
					_ = response.Body.Close()
				}()

				res, err := io.ReadAll(response.Body)

				if err != nil {
					// skip error
					return token2022Info, tokenInfo, err
				}
				// 检查 Content-Type
				contentType := response.Header.Get("Content-Type")
				if strings.Contains(string(res), "Account has been disabled.") {
					return token2022Info, tokenInfo, nil
				}
				if strings.HasPrefix(contentType, "application/json") {
					tokenUriData, err := Byte2Struct[entity.TokenUriData](res)
					if err != nil {
						return token2022Info, tokenInfo, nil
					}

					if len(tokenUriData.Website) == 0 {
						tokenUriData.Website = tokenUriData.Website
					}
					if len(tokenUriData.Telegram) == 0 {
						tokenUriData.Telegram = tokenUriData.Telegram
					}
					if len(tokenUriData.Twitter) == 0 {
						tokenUriData.Twitter = tokenUriData.Twitter
					}

					tokenInfo.Uri = tokenUriData
				} else if strings.HasPrefix(contentType, "image/") {
					// maybe picture
					// https://solscan.io/token/2HPtzSqkivqk8P5ySqVxB17b93sXsJN4s77kJp4Eish9#metadata
					// if strings.Contains(err.Error(), "invalid character") {
					// 	tokenInfo.Uri.Image = metaData.Data.Uri
					// }
					// skip error
					tokenInfo.Uri.Image = extension.State.Uri
				} else {

					tokenUriData, err := Byte2Struct[entity.TokenUriData](res)
					if err != nil {
						err = fmt.Errorf("GetToken2022Info error: %v, url: %v, token address: %v", err, extension.State.Uri, address)
						return token2022Info, tokenInfo, err
					}

					if len(tokenUriData.Website) == 0 {
						tokenUriData.Website = tokenUriData.Website
					}
					if len(tokenUriData.Telegram) == 0 {
						tokenUriData.Telegram = tokenUriData.Telegram
					}
					if len(tokenUriData.Twitter) == 0 {
						tokenUriData.Twitter = tokenUriData.Twitter
					}

					tokenInfo.Uri = tokenUriData
				}

			}
		}
	}
	return &mintResponse.Parsed.Info, tokenInfo, nil
}

func Byte2Struct[T any](data []byte) (T, error) {
	var t T
	err := json.Unmarshal(data, &t)
	return t, err
}

// 检查 URL 是否可访问
func isURLAccessible(url string) bool {
	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// 替换为公共的 IPFS 网关
func replaceWithPublicGateway(ipfsURL string, publicGateway string) string {
	// 正则表达式来匹配 IPFS 网关
	pattern := `^https?://[^/]+/ipfs/`

	re := regexp.MustCompile(pattern)
	if re.MatchString(ipfsURL) {
		// 替换为公共网关
		return re.ReplaceAllString(ipfsURL, publicGateway)
	}
	return ipfsURL // 如果没有匹配，返回原始 URL
}

func NewTokenService(sc *svc.ServiceContext) TokenService {
	ctx, cancel := context.WithCancelCause(context.Background())
	rpcURL := strings.Replace(sc.Config.Helius.WSUrl, "wss://", "https://", 1)
	return &TokenServiceImpl{
		ctx:    ctx,
		cancel: cancel,
		sc:     sc,
		Logger: logx.WithContext(ctx).WithFields(logx.Field("service", "token")),
		rpcUrl: rpcURL,
		c: client.New(rpc.WithEndpoint(rpcURL), rpc.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
				DialContext: (&net.Dialer{
					Timeout: 5 * time.Second,
				}).DialContext,
			},
		})),
	}
}
