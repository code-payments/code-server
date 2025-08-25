package currency

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	currencypb "github.com/code-payments/code-protobuf-api/generated/go/currency/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	currency_util "github.com/code-payments/code-server/pkg/code/currency"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/currencycreator"
	"github.com/code-payments/code-server/pkg/solana/token"
)

type currencyServer struct {
	log  *logrus.Entry
	data code_data.Provider

	currencypb.UnimplementedCurrencyServer
}

func NewCurrencyServer(
	data code_data.Provider,
) currencypb.CurrencyServer {
	return &currencyServer{
		log:  logrus.StandardLogger().WithField("type", "currency/server"),
		data: data,
	}
}

func (s *currencyServer) GetAllRates(ctx context.Context, req *currencypb.GetAllRatesRequest) (resp *currencypb.GetAllRatesResponse, err error) {
	log := s.log.WithField("method", "GetAllRates")
	log = client.InjectLoggingMetadata(ctx, log)

	var record *currency.MultiRateRecord
	if req.Timestamp != nil && req.Timestamp.AsTime().Before(time.Now().Add(-15*time.Minute)) {
		record, err = s.LoadExchangeRatesForTime(ctx, req.Timestamp.AsTime())
	} else if req.Timestamp == nil || req.Timestamp.AsTime().Sub(time.Now()) < time.Hour {
		record, err = s.LoadExchangeRatesLatest(ctx)
	} else {
		return nil, status.Error(codes.InvalidArgument, "timestamp too far in the future")
	}

	if err != nil {
		log.WithError(err).Warn("failed to load latest rate")
		return nil, status.Error(codes.Internal, err.Error())
	}

	protoTime := timestamppb.New(record.Time)
	return &currencypb.GetAllRatesResponse{
		AsOf:  protoTime,
		Rates: record.Rates,
	}, nil
}

func (s *currencyServer) GetMints(ctx context.Context, req *currencypb.GetMintsRequest) (*currencypb.GetMintsResponse, error) {
	log := s.log.WithField("method", "GetMints")
	log = client.InjectLoggingMetadata(ctx, log)

	resp := &currencypb.GetMintsResponse{}

	for _, protoMintAddress := range req.Addresses {
		mintAccount, err := common.NewAccountFromProto(protoMintAddress)
		if err != nil {
			log.WithError(err).Warn("Invalid mint address")
			return nil, status.Error(codes.Internal, "")
		}

		log = log.WithField("mint", mintAccount.PublicKey().ToBase58())

		var protoMetadata *currencypb.Mint
		switch mintAccount.PublicKey().ToBase58() {
		case common.CoreMintAccount.PublicKey().ToBase58():
			protoMetadata = &currencypb.Mint{
				Address:  protoMintAddress,
				Decimals: uint32(common.CoreMintDecimals),
				Name:     common.CoreMintName,
				Symbol:   strings.ToUpper(string(common.CoreMintSymbol)),
				VmMetadata: &currencypb.VmMintMetadata{
					Vm:                 common.CodeVmAccount.ToProto(),
					Authority:          common.GetSubsidizer().ToProto(),
					LockDurationInDays: 21,
				},
			}
		case "52MNGpgvydSwCtC2H4qeiZXZ1TxEuRVCRGa8LAfk2kSj": // todo: load from DB populated by worker
			authorityAccount, _ := common.NewAccountFromPublicKeyString("jfy1btcfsjSn2WCqLVaxiEjp4zgmemGyRsdCPbPwnZV")
			jeffyVaultAccount, _ := common.NewAccountFromPublicKeyString("BFDanLgELhpCCGTtaa7c8WGxTXcTxgwkf9DMQd4qheSK")
			coreMintVaultAccount, _ := common.NewAccountFromPublicKeyString("A9NVHVuorNL4y2YFxdwdU3Hqozxw1Y1YJ81ZPxJsRrT4")
			vmAccount, _ := common.NewAccountFromPublicKeyString("Bii3UFB9DzPq6UxgewF5iv9h1Gi8ZnP6mr7PtocHGNta")

			var tokenAccount token.Account
			ai, err := s.data.GetBlockchainAccountInfo(ctx, jeffyVaultAccount.PublicKey().ToBase58(), solana.CommitmentFinalized)
			if err != nil {
				log.Warn("Failure getting Jeffy vault balance")
				return nil, status.Error(codes.Internal, "")
			}
			tokenAccount.Unmarshal(ai.Data)
			jeffyVaultBalance := tokenAccount.Amount

			ai, err = s.data.GetBlockchainAccountInfo(ctx, coreMintVaultAccount.PublicKey().ToBase58(), solana.CommitmentFinalized)
			if err != nil {
				log.Warn("Failure getting USDC vault balance")
				return nil, status.Error(codes.Internal, "")
			}
			tokenAccount.Unmarshal(ai.Data)
			coreMintVaultBalance := tokenAccount.Amount

			protoMetadata = &currencypb.Mint{
				Address:  protoMintAddress,
				Decimals: currencycreator.DefaultMintDecimals,
				Name:     "Jeffy",
				Symbol:   "JFY",
				VmMetadata: &currencypb.VmMintMetadata{
					Vm:                 vmAccount.ToProto(),
					Authority:          authorityAccount.ToProto(),
					LockDurationInDays: 21,
				},
				CurrencyCreatorMetadata: &currencypb.CurrencyCreatorMintMetadata{
					SupplyFromBonding:    currencycreator.DefaultMintMaxQuarkSupply - jeffyVaultBalance,
					CoreMintTokensLocked: coreMintVaultBalance,
					BuyFeeBps:            currencycreator.DefaultBuyFeeBps,
					SellFeeBps:           currencycreator.DefaultSellFeeBps,
				},
			}
		default:
			return &currencypb.GetMintsResponse{Result: currencypb.GetMintsResponse_NOT_FOUND}, nil
		}

		resp.MetadataByAddress[common.CoreMintAccount.PublicKey().ToBase58()] = protoMetadata
	}
	return &currencypb.GetMintsResponse{}, nil
}

func (s *currencyServer) LoadExchangeRatesForTime(ctx context.Context, t time.Time) (*currency.MultiRateRecord, error) {
	record, err := s.data.GetAllExchangeRates(ctx, t)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get price record by date")
	}
	return record, nil
}

func (s *currencyServer) LoadExchangeRatesLatest(ctx context.Context) (*currency.MultiRateRecord, error) {
	latest, err := s.data.GetAllExchangeRates(ctx, currency_util.GetLatestExchangeRateTime())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get latest price record")
	}
	return latest, nil
}
