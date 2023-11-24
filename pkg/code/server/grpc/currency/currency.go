package currency

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	currencypb "github.com/code-payments/code-protobuf-api/generated/go/currency/v1"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	exchange_rate_util "github.com/code-payments/code-server/pkg/code/exchangerate"
	"github.com/code-payments/code-server/pkg/grpc/client"
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

func (s *currencyServer) LoadExchangeRatesForTime(ctx context.Context, t time.Time) (*currency.MultiRateRecord, error) {
	record, err := s.data.GetAllExchangeRates(ctx, t)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get price record by date")
	}
	return record, nil
}

func (s *currencyServer) LoadExchangeRatesLatest(ctx context.Context) (*currency.MultiRateRecord, error) {
	latest, err := s.data.GetAllExchangeRates(ctx, exchange_rate_util.GetLatestExchangeRateTime())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get latest price record")
	}
	return latest, nil
}
