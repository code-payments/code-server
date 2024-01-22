package transaction_v2

import (
	"context"
	"sync"

	"github.com/oschwald/maxminddb-golang"
	"github.com/sirupsen/logrus"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/antispam"
	auth_util "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/lawenforcement"
	"github.com/code-payments/code-server/pkg/code/server/grpc/messaging"
	"github.com/code-payments/code-server/pkg/jupiter"
	"github.com/code-payments/code-server/pkg/kin"
	push_lib "github.com/code-payments/code-server/pkg/push"
	sync_util "github.com/code-payments/code-server/pkg/sync"
)

type transactionServer struct {
	log  *logrus.Entry
	conf *conf

	data code_data.Provider

	auth *auth_util.RPCSignatureVerifier

	pusher push_lib.Provider

	jupiterClient *jupiter.Client

	messagingClient messaging.InternalMessageClient

	maxmind       *maxminddb.Reader
	antispamGuard *antispam.Guard
	amlGuard      *lawenforcement.AntiMoneyLaunderingGuard

	// todo: A better way of managing this if/when we do a treasury per individual transaction amount
	treasuryPoolNameByBaseAmount map[uint64]string

	treasuryPoolStatsLock                sync.RWMutex
	treasuryPoolStatsInitialLoadWgByName map[string]*sync.WaitGroup
	currentTreasuryPoolStatsByName       map[string]*treasuryPoolStats

	// todo: distributed locks
	intentLocks   *sync_util.StripedLock
	ownerLocks    *sync_util.StripedLock
	giftCardLocks *sync_util.StripedLock
	phoneLocks    *sync_util.StripedLock

	airdropperLock sync.Mutex
	airdropper     *common.TimelockAccounts

	swapSubsidizer *common.Account

	feeCollector *common.Account

	transactionpb.UnimplementedTransactionServer
}

func NewTransactionServer(
	data code_data.Provider,
	pusher push_lib.Provider,
	jupiterClient *jupiter.Client,
	messagingClient messaging.InternalMessageClient,
	maxmind *maxminddb.Reader,
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

		pusher: pusher,

		jupiterClient: jupiterClient,

		messagingClient: messagingClient,

		maxmind:       maxmind,
		antispamGuard: antispamGuard,
		amlGuard:      lawenforcement.NewAntiMoneyLaunderingGuard(data),

		intentLocks:   sync_util.NewStripedLock(stripedLockParallelization),
		ownerLocks:    sync_util.NewStripedLock(stripedLockParallelization),
		giftCardLocks: sync_util.NewStripedLock(stripedLockParallelization),
		phoneLocks:    sync_util.NewStripedLock(stripedLockParallelization),
	}

	s.treasuryPoolNameByBaseAmount = map[uint64]string{
		kin.ToQuarks(1):         s.conf.treasuryPoolOneKinBucket.Get(ctx),
		kin.ToQuarks(10):        s.conf.treasuryPoolTenKinBucket.Get(ctx),
		kin.ToQuarks(100):       s.conf.treasuryPoolHundredKinBucket.Get(ctx),
		kin.ToQuarks(1_000):     s.conf.treasuryPoolThousandKinBucket.Get(ctx),
		kin.ToQuarks(10_000):    s.conf.treasuryPoolTenThousandKinBucket.Get(ctx),
		kin.ToQuarks(100_000):   s.conf.treasuryPoolHundredThousandKinBucket.Get(ctx),
		kin.ToQuarks(1_000_000): s.conf.treasuryPoolMillionKinBucket.Get(ctx),
	}
	s.currentTreasuryPoolStatsByName = make(map[string]*treasuryPoolStats)
	s.treasuryPoolStatsInitialLoadWgByName = make(map[string]*sync.WaitGroup)

	for _, treasury := range s.treasuryPoolNameByBaseAmount {
		var wg sync.WaitGroup
		wg.Add(1)

		s.treasuryPoolStatsLock.Lock()
		s.treasuryPoolStatsInitialLoadWgByName[treasury] = &wg
		s.treasuryPoolStatsLock.Unlock()

		go s.treasuryPoolMonitor(ctx, treasury)
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
