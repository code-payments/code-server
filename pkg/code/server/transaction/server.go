package transaction_v2

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/aml"
	"github.com/code-payments/code-server/pkg/code/antispam"
	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/transaction"
	sync_util "github.com/code-payments/code-server/pkg/sync"
)

type transactionServer struct {
	log  *logrus.Entry
	conf *conf

	data code_data.Provider

	auth *auth_util.RPCSignatureVerifier

	airdropIntegration AirdropIntegration

	antispamGuard *antispam.Guard
	amlGuard      *aml.Guard

	noncePool *transaction.LocalNoncePool

	airdropperLock sync.Mutex
	airdropper     *common.TimelockAccounts

	feeCollector *common.Account

	ownerLocks    *sync_util.StripedLock
	giftCardLocks *sync_util.StripedLock

	transactionpb.UnimplementedTransactionServer
}

func NewTransactionServer(
	data code_data.Provider,
	airdropIntegration AirdropIntegration,
	antispamGuard *antispam.Guard,
	amlGuard *aml.Guard,
	noncePool *transaction.LocalNoncePool,
	configProvider ConfigProvider,
) (transactionpb.TransactionServer, error) {
	var err error

	ctx := context.Background()

	conf := configProvider()

	stripedLockParallelization := uint(conf.stripedLockParallelization.Get(ctx))

	if err := noncePool.Validate(nonce.EnvironmentCvm, common.CodeVmAccount.PublicKey().ToBase58(), nonce.PurposeClientTransaction); err != nil {
		return nil, err
	}

	s := &transactionServer{
		log:  logrus.StandardLogger().WithField("type", "transaction/v2/server"),
		conf: conf,

		data: data,

		auth: auth_util.NewRPCSignatureVerifier(data),

		airdropIntegration: airdropIntegration,

		antispamGuard: antispamGuard,
		amlGuard:      amlGuard,

		noncePool: noncePool,

		ownerLocks:    sync_util.NewStripedLock(stripedLockParallelization),
		giftCardLocks: sync_util.NewStripedLock(stripedLockParallelization),
	}

	s.feeCollector, err = common.NewAccountFromPublicKeyString(s.conf.feeCollectorTokenPublicKey.Get(ctx))
	if err != nil {
		return nil, err
	}

	airdropper := s.conf.airdropperOwnerPublicKey.Get(ctx)
	if len(airdropper) > 0 && airdropper != defaultAirdropperOwnerPublicKey {
		err := s.loadAirdropper(ctx)
		if err != nil {
			return nil, err
		}
	}

	return s, nil
}
