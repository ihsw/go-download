package state

import (
	"fmt"

	"github.com/sotah-inc/server/app/pkg/database"
	"github.com/sotah-inc/server/app/pkg/diskstore"
	"github.com/sotah-inc/server/app/pkg/logging"
	"github.com/sotah-inc/server/app/pkg/messenger"
	"github.com/sotah-inc/server/app/pkg/metric"
	"github.com/sotah-inc/server/app/pkg/state/subjects"
	"github.com/sotah-inc/server/app/pkg/util"
	"github.com/twinj/uuid"
)

type LiveAuctionsStateConfig struct {
	MessengerHost string
	MessengerPort int

	DiskStoreCacheDir string

	LiveAuctionsDatabaseDir string
}

func NewLiveAuctionsState(config LiveAuctionsStateConfig) (LiveAuctionsState, error) {
	laState := LiveAuctionsState{
		State: NewState(uuid.NewV4(), false),
	}

	// connecting to the messenger host
	logging.Info("Connecting messenger")
	mess, err := messenger.NewMessenger(config.MessengerHost, config.MessengerPort)
	if err != nil {
		return LiveAuctionsState{}, err
	}
	laState.IO.Messenger = mess

	// initializing a reporter
	laState.IO.Reporter = metric.NewReporter(mess)

	// gathering regions
	logging.Info("Gathering regions")
	regions, err := laState.NewRegions()
	if err != nil {
		return LiveAuctionsState{}, err
	}
	laState.Regions = regions

	// gathering statuses
	logging.Info("Gathering statuses")
	for _, reg := range laState.Regions {
		status, err := laState.NewStatus(reg)
		if err != nil {
			return LiveAuctionsState{}, err
		}

		laState.Statuses[reg.Name] = status
	}

	// ensuring database paths exist
	databasePaths := []string{}
	for regionName, status := range laState.Statuses {
		for _, realm := range status.Realms {
			databasePaths = append(databasePaths, fmt.Sprintf(
				"%s/live-auctions/%s/%s",
				config.LiveAuctionsDatabaseDir,
				regionName,
				realm.Slug,
			))
		}
	}
	if err := util.EnsureDirsExist(databasePaths); err != nil {
		return LiveAuctionsState{}, err
	}

	// establishing a store
	logging.Info("Connecting to disk store")
	cacheDirs := []string{
		config.DiskStoreCacheDir,
		fmt.Sprintf("%s/auctions", config.DiskStoreCacheDir),
	}
	for _, reg := range laState.Regions {
		cacheDirs = append(cacheDirs, fmt.Sprintf("%s/auctions/%s", config.DiskStoreCacheDir, reg.Name))
	}
	if err := util.EnsureDirsExist(cacheDirs); err != nil {
		return LiveAuctionsState{}, err
	}
	laState.IO.DiskStore = diskstore.NewDiskStore(config.DiskStoreCacheDir)

	// loading the live-auctions databases
	logging.Info("Connecting to live-auctions databases")
	ladBases, err := database.NewLiveAuctionsDatabases(config.LiveAuctionsDatabaseDir, laState.Statuses)
	if err != nil {
		return LiveAuctionsState{}, err
	}
	laState.IO.Databases.LiveAuctionsDatabases = ladBases

	// establishing listeners
	laState.Listeners = NewListeners(SubjectListeners{
		subjects.Auctions:           laState.ListenForAuctions,
		subjects.LiveAuctionsIntake: laState.ListenForLiveAuctionsIntake,
		subjects.PriceList:          laState.ListenForPriceList,
		subjects.Owners:             laState.ListenForOwners,
		subjects.OwnersQuery:        laState.ListenForOwnersQuery,
		subjects.OwnersQueryByItems: laState.ListenForOwnersQueryByItems,
	})

	return laState, nil
}

type LiveAuctionsState struct {
	State
}
