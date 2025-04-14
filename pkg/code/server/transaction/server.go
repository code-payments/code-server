package transaction_v2

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/antispam"
	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/lawenforcement"
	"github.com/code-payments/code-server/pkg/code/server/messaging"
	"github.com/code-payments/code-server/pkg/jupiter"
	sync_util "github.com/code-payments/code-server/pkg/sync"
)

type transactionServer struct {
	log  *logrus.Entry
	conf *conf

	data code_data.Provider

	auth *auth_util.RPCSignatureVerifier

	airdropIntegration AirdropIntegration

	jupiterClient *jupiter.Client

	messagingClient messaging.InternalMessageClient

	antispamGuard *antispam.Guard
	amlGuard      *lawenforcement.AntiMoneyLaunderingGuard

	// todo: distributed locks
	intentLocks   *sync_util.StripedLock
	ownerLocks    *sync_util.StripedLock
	giftCardLocks *sync_util.StripedLock

	airdropperLock sync.Mutex
	airdropper     *common.TimelockAccounts

	swapSubsidizer *common.Account

	feeCollector *common.Account

	transactionpb.UnimplementedTransactionServer
}

func NewTransactionServer(
	data code_data.Provider,
	airdropIntegration AirdropIntegration,
	jupiterClient *jupiter.Client,
	messagingClient messaging.InternalMessageClient,
	antispamGuard *antispam.Guard,
	configProvider ConfigProvider,
) transactionpb.TransactionServer {
	ctx := context.Background()

	conf := configProvider()

	stripedLockParallelization := uint(conf.stripedLockParallelization.Get(ctx))

	s := &transactionServer{
		log:  logrus.StandardLogger().WithField("type", "transaction/v2/server"),
		conf: conf,

		data: data,

		auth: auth_util.NewRPCSignatureVerifier(data),

		airdropIntegration: airdropIntegration,

		jupiterClient: jupiterClient,

		messagingClient: messagingClient,

		antispamGuard: antispamGuard,
		amlGuard:      lawenforcement.NewAntiMoneyLaunderingGuard(data),

		intentLocks:   sync_util.NewStripedLock(stripedLockParallelization),
		ownerLocks:    sync_util.NewStripedLock(stripedLockParallelization),
		giftCardLocks: sync_util.NewStripedLock(stripedLockParallelization),
	}

	airdropper := s.conf.airdropperOwnerPublicKey.Get(ctx)
	if len(airdropper) > 0 && airdropper != defaultAirdropperOwnerPublicKey {
		s.mustLoadAirdropper(ctx)
	}

	swapSubsidizer := s.conf.swapSubsidizerOwnerPublicKey.Get(ctx)
	if len(swapSubsidizer) > 0 && swapSubsidizer != defaultSwapSubsidizerOwnerPublicKey {
		s.mustLoadSwapSubsidizer(ctx)
	}

	feeCollector, err := common.NewAccountFromPublicKeyString(conf.feeCollectorTokenPublicKey.Get(ctx))
	if err != nil {
		s.log.WithError(err).Fatal("failure loading fee collector account")
	}
	s.feeCollector = feeCollector

	return s
}
