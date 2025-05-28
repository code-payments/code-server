package transaction_v2

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/aml"
	"github.com/code-payments/code-server/pkg/code/antispam"
	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/jupiter"
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

	// Not configured, since micropeayments require new implementation and are disabled
	feeCollector *common.Account

	// Not configured, since swaps require new implementation and are disabled
	jupiterClient  *jupiter.Client
	swapSubsidizer *common.Account

	// todo: distributed locks
	intentLocks   *sync_util.StripedLock
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
	ctx := context.Background()

	conf := configProvider()

	stripedLockParallelization := uint(conf.stripedLockParallelization.Get(ctx))

	noncePoolEnv, noncePoolEnvInstance, noncePoolType := noncePool.GetConfiguration()
	if noncePoolEnv != nonce.EnvironmentCvm {
		return nil, errors.Errorf("nonce pool environment must be %s", nonce.EnvironmentCvm)
	}
	if noncePoolEnvInstance != common.CodeVmAccount.PublicKey().ToBase58() {
		return nil, errors.Errorf("nonce pool environment instance must be %s", common.CodeVmAccount.PublicKey().ToBase58())
	}
	if noncePoolType != nonce.PurposeClientTransaction {
		return nil, errors.Errorf("nonce pool type must be %s", nonce.PurposeClientTransaction)
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

		intentLocks:   sync_util.NewStripedLock(stripedLockParallelization),
		ownerLocks:    sync_util.NewStripedLock(stripedLockParallelization),
		giftCardLocks: sync_util.NewStripedLock(stripedLockParallelization),
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
