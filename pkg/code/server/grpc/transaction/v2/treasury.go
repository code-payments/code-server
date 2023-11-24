package transaction_v2

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
)

// todo: any other treasury-related things we can put here?

func init() {
	cachedTreasuryMetadatas = make(map[string]*cachedTreasuryMetadata)
}

var (
	treasuryCacheMu         sync.RWMutex
	cachedTreasuryMetadatas map[string]*cachedTreasuryMetadata
)

type cachedTreasuryMetadata struct {
	name string

	stateAccount *common.Account
	stateBump    uint8

	vaultAccount *common.Account
	vaultBump    uint8

	mostRecentRoot string

	lastUpdatedAt time.Time
}

func getCachedTreasuryMetadataByNameOrAddress(ctx context.Context, data code_data.Provider, nameOrAddress string, maxAge time.Duration) (*cachedTreasuryMetadata, error) {
	treasuryCacheMu.RLock()
	cached, ok := cachedTreasuryMetadatas[nameOrAddress]
	if ok && time.Since(cached.lastUpdatedAt) < maxAge {
		treasuryCacheMu.RUnlock()
		return cached, nil
	}
	treasuryCacheMu.RUnlock()

	treasuryCacheMu.Lock()
	defer treasuryCacheMu.Unlock()

	cached, ok = cachedTreasuryMetadatas[nameOrAddress]
	if ok && time.Since(cached.lastUpdatedAt) < maxAge {
		return cached, nil
	}

	var isName bool
	_, err := common.NewAccountFromPublicKeyString(nameOrAddress)
	if err != nil {
		isName = true
	}

	var treasuryPoolRecord *treasury.Record
	if isName {
		treasuryPoolRecord, err = data.GetTreasuryPoolByName(ctx, nameOrAddress)
		if err != nil {
			return nil, err
		}
	} else {
		treasuryPoolRecord, err = data.GetTreasuryPoolByAddress(ctx, nameOrAddress)
		if err != nil {
			return nil, err
		}
	}

	stateAccount, err := common.NewAccountFromPublicKeyString(treasuryPoolRecord.Address)
	if err != nil {
		return nil, err
	}

	vaultAccount, err := common.NewAccountFromPublicKeyString(treasuryPoolRecord.Vault)
	if err != nil {
		return nil, err
	}

	cached = &cachedTreasuryMetadata{
		name: treasuryPoolRecord.Name,

		stateAccount: stateAccount,
		stateBump:    treasuryPoolRecord.Bump,

		vaultAccount: vaultAccount,
		vaultBump:    treasuryPoolRecord.VaultBump,

		mostRecentRoot: treasuryPoolRecord.GetMostRecentRoot(),

		lastUpdatedAt: time.Now(),
	}
	cachedTreasuryMetadatas[treasuryPoolRecord.Name] = cached
	cachedTreasuryMetadatas[treasuryPoolRecord.Address] = cached
	return cached, nil
}

type treasuryPoolStats struct {
	totalAvailableFunds uint64
	totalDeficit        uint64
	lastUpdatedAt       time.Time
}

// todo: Assumes constant and single treasury pool
func (s *transactionServer) treasuryPoolMonitor(ctx context.Context, name string) {
	log := s.log.WithFields(logrus.Fields{
		"method":   "treasuryPoolMonitor",
		"treasury": name,
	})

	var initialLoadCompleted bool
	var treasuryPoolRecord *treasury.Record
	var err error

	for {
		func() {
			ctx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()

			start := time.Now()
			defer time.Sleep(s.conf.treasuryPoolStatsRefreshInterval.Get(ctx) - time.Since(start))

			if treasuryPoolRecord == nil {
				treasuryPoolRecord, err = s.data.GetTreasuryPoolByName(ctx, name)
				if err != nil {
					log.WithError(err).Warn("failure getting treasury pool record")
					return
				}
			}

			totalAvailable, err := s.data.GetTotalAvailableTreasuryPoolFunds(ctx, treasuryPoolRecord.Vault)
			if err != nil {
				log.WithError(err).Warn("failure getting total funds")
				return
			}

			totalDeficit, err := s.data.GetTotalTreasuryPoolDeficitFromCommitments(ctx, treasuryPoolRecord.Address)
			if err != nil {
				log.WithError(err).Warn("failure getting total deficit")
				return
			}

			s.treasuryPoolStatsLock.Lock()
			defer s.treasuryPoolStatsLock.Unlock()

			s.currentTreasuryPoolStatsByName[name] = &treasuryPoolStats{
				totalAvailableFunds: totalAvailable,
				totalDeficit:        totalDeficit,
				lastUpdatedAt:       time.Now(),
			}

			if !initialLoadCompleted {
				initialLoadCompleted = true
				s.treasuryPoolStatsInitialLoadWgByName[name].Done()
			}
		}()
	}
}

func (s *transactionServer) selectTreasuryPoolForAdvance(ctx context.Context, quarks uint64) (string, error) {
	log := s.log.WithFields(logrus.Fields{
		"method": "selectTreasuryPoolForAdvance",
		"quarks": quarks,
	})

	for _, bucket := range []uint64{
		kin.ToQuarks(1_000_000),
		kin.ToQuarks(100_000),
		kin.ToQuarks(10_000),
		kin.ToQuarks(1_000),
		kin.ToQuarks(100),
		kin.ToQuarks(10),
		kin.ToQuarks(1),
	} {
		// Not a multiple of the bucket
		if quarks%bucket != 0 {
			continue
		}

		// Sanity check for something too large to allow
		if quarks > 9*bucket {
			continue
		}

		return s.treasuryPoolNameByBaseAmount[bucket], nil
	}

	// If we reach this point, we must have allowed a bucket multiple that wasn't really allowed
	log.Warn("no treasury selected to handle advance")

	return "", errors.New("treasury not available")
}

func (s *transactionServer) areAllTreasuryPoolsAvailable(ctx context.Context) (bool, error) {
	for _, treasuryPoolName := range s.treasuryPoolNameByBaseAmount {
		isTreasuryAvailable, err := s.isTreasuryPoolAvailable(ctx, treasuryPoolName)
		if err != nil {
			return false, err
		} else if !isTreasuryAvailable {
			return false, nil
		}
	}
	return true, nil
}

func (s *transactionServer) isTreasuryPoolAvailable(ctx context.Context, name string) (bool, error) {
	log := s.log.WithFields(logrus.Fields{
		"method":   "isTreasuryPoolAvailable",
		"treasury": name,
	})

	s.treasuryPoolStatsInitialLoadWgByName[name].Wait()

	s.treasuryPoolStatsLock.RLock()
	defer s.treasuryPoolStatsLock.RUnlock()

	currentStats := s.currentTreasuryPoolStatsByName[name]

	// Have a small threshold where we allow stats to be outdated to allow for blips
	if time.Since(currentStats.lastUpdatedAt) >= time.Minute {
		log.Warn("treasury pool stats are outdated")
		return false, errors.New("treasury pool stats are outdated")
	}

	if currentStats.totalDeficit >= currentStats.totalAvailableFunds {
		log.Warn("treasury pool isn't available because all funds are accounted for")
		return false, nil
	}
	remaining := currentStats.totalAvailableFunds - currentStats.totalDeficit

	// Try to maintain some available funds to avoid any potential deadlocks.
	// If we happen to go slightly over, it's not the end of the world either.
	percentAvailable := float64(remaining) / float64(currentStats.totalAvailableFunds)
	if percentAvailable < 0.05 {
		log.Warn("treasury pool isn't available because most funds are accounted for")
		return false, nil
	}

	return true, nil
}
