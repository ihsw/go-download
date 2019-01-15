package main

import (
	"encoding/json"
	"time"

	"github.com/sotah-inc/server/app/metric"

	"github.com/sotah-inc/server/app/blizzard"
	"github.com/sotah-inc/server/app/logging"

	nats "github.com/nats-io/go-nats"
	"github.com/sirupsen/logrus"
	"github.com/sotah-inc/server/app/subjects"
)

func newAuctionsIntakeRequest(payload []byte) (auctionsIntakeRequest, error) {
	ar := &auctionsIntakeRequest{}
	err := json.Unmarshal(payload, &ar)
	if err != nil {
		return auctionsIntakeRequest{}, err
	}

	return *ar, nil
}

type realmMapValue struct {
	realm        realm
	lastModified time.Time
}

type realmMap struct {
	values map[blizzard.RealmSlug]realmMapValue
}

func (rMap realmMap) toRealms() realms {
	out := realms{}
	for _, rValue := range rMap.values {
		out = append(out, rValue.realm)
	}

	return out
}

type regionRealmMap = map[regionName]realmMap

type intakeRequestData = map[regionName]map[blizzard.RealmSlug]int64

type auctionsIntakeRequest struct {
	RegionRealmTimestamps intakeRequestData `json:"region_realm_timestamps"`
}

func (aiRequest auctionsIntakeRequest) resolve(sta state) (regionRealmMap, regionRealmMap, error) {
	c := sta.resolver.config

	includedRegionRealms := regionRealmMap{}
	excludedRegionRealms := regionRealmMap{}
	for _, reg := range sta.regions {
		includedRegionRealms[reg.Name] = realmMap{map[blizzard.RealmSlug]realmMapValue{}}

		excludedRegionRealms[reg.Name] = realmMap{map[blizzard.RealmSlug]realmMapValue{}}
		for _, rea := range sta.statuses[reg.Name].Realms {
			excludedRegionRealms[reg.Name].values[rea.Slug] = realmMapValue{rea, time.Time{}}
		}
	}

	for rName, realmSlugs := range aiRequest.RegionRealmTimestamps {
		for realmSlug, unixTimestamp := range realmSlugs {
			for _, rea := range sta.statuses[rName].Realms.filterWithWhitelist(c.Whitelist[rName]) {
				if rea.Slug != realmSlug {
					continue
				}

				includedRegionRealms[rName].values[realmSlug] = realmMapValue{rea, time.Unix(unixTimestamp, 0)}
				delete(excludedRegionRealms[rName].values, realmSlug)
			}
		}
	}

	return includedRegionRealms, excludedRegionRealms, nil
}

func (aiRequest auctionsIntakeRequest) handle(sta state) {
	// resolving included and excluded region realms
	includedRegionRealms, excludedRegionRealms, err := aiRequest.resolve(sta)
	if err != nil {
		logging.WithField("error", err.Error()).Info("Failed to resolve auctions-intake-request")

		return
	}

	// misc for metrics
	totalRealms := 0
	for rName, reas := range sta.statuses {
		totalRealms += len(reas.Realms.filterWithWhitelist(sta.resolver.config.Whitelist[rName]))
	}
	includedRealmCount := 0
	for _, reas := range includedRegionRealms {
		includedRealmCount += len(reas.values)
	}
	excludedRealmCount := 0
	for _, reas := range excludedRegionRealms {
		excludedRealmCount += len(reas.values)
	}

	logging.WithFields(logrus.Fields{
		"included_realms": includedRealmCount,
		"excluded_realms": excludedRealmCount,
		"total_realms":    totalRealms,
	}).Info("Handling auctions-intake-request")

	// misc
	startTime := time.Now()

	// loading in auctions from region-realms
	for rName, rMap := range includedRegionRealms {
		logging.WithFields(logrus.Fields{
			"region": rName,
			"realms": len(rMap.values),
		}).Debug("Going over realms to load auctions")

		// loading auctions from file cache or gcloud store
		loadedAuctions := func() chan loadAuctionsJob {
			if sta.resolver.config.UseGCloudStorage {
				return sta.resolver.store.loadRegionRealmMap(rMap)
			}

			return rMap.toRealms().loadAuctionsFromCacheDir(sta.resolver.config)
		}()
		done := sta.pricelistHistoryDatabases.load(loadedAuctions, *sta.resolver.config, sta.resolver.store)
		<-done

		logging.WithFields(logrus.Fields{
			"region": rName,
			"realms": len(rMap.values),
		}).Debug("Finished loading auctions")
	}

	metric.ReportDuration(
		metric.PricelistsIntakeDuration,
		metric.DurationMetrics{
			Duration:       time.Now().Sub(startTime),
			TotalRealms:    totalRealms,
			IncludedRealms: includedRealmCount,
			ExcludedRealms: excludedRealmCount,
		},
		logrus.Fields{},
	)
}

func (sta state) listenForAuctionsIntake(stop listenStopChan) error {
	// spinning up a worker for handling auctions-intake requests
	in := make(chan auctionsIntakeRequest, 10)
	go func() {
		for {
			aiRequest := <-in

			// misc
			startTime := time.Now()

			includedRegionRealms, excludedRegionRealms, err := aiRequest.resolve(sta)
			if err != nil {
				logging.WithField("error", err.Error()).Error("Failed to resolve auctions-intake-request")

				continue
			}

			totalRealms := 0
			for rName, reas := range sta.statuses {
				totalRealms += len(reas.Realms.filterWithWhitelist(sta.resolver.config.Whitelist[rName]))
			}
			includedRealmCount := 0
			for _, reas := range includedRegionRealms {
				includedRealmCount += len(reas.values)
			}
			excludedRealmCount := 0
			for _, reas := range excludedRegionRealms {
				excludedRealmCount += len(reas.values)
			}

			logging.WithFields(logrus.Fields{
				"included_realms": includedRealmCount,
				"excluded_realms": excludedRealmCount,
				"total_realms":    totalRealms,
			}).Info("Handling auctions-intake-request")

			// metrics
			totalPreviousAuctions := 0
			totalRemovedAuctions := 0
			totalNewAuctions := 0
			totalAuctions := 0
			totalOwners := 0
			currentItemIds := map[blizzard.ItemID]struct{}{}

			// gathering the total number of auctions pre-intake
			logging.Info("Going over all auctions to for pre-intake metrics")
			for statsJob := range sta.liveAuctionsDatabases.getStats(nil) {
				if statsJob.err != nil {
					logging.WithFields(logrus.Fields{
						"error":  statsJob.err.Error(),
						"region": statsJob.realm.region.Name,
						"realm":  statsJob.realm.Slug,
					}).Error("Failed to fetch stats from live-auctions database")

					continue
				}

				totalPreviousAuctions += statsJob.stats.totalAuctions
			}
			for statsJob := range sta.liveAuctionsDatabases.getStats(excludedRegionRealms) {
				if statsJob.err != nil {
					logging.WithFields(logrus.Fields{
						"error":  statsJob.err.Error(),
						"region": statsJob.realm.region.Name,
						"realm":  statsJob.realm.Slug,
					}).Error("Failed to fetch stats from live-auctions database")

					continue
				}

				totalAuctions += statsJob.stats.totalAuctions
				totalOwners += len(statsJob.stats.ownerNames)
				for _, ID := range statsJob.stats.itemIds {
					currentItemIds[ID] = struct{}{}
				}
			}

			// going over auctions in the filecache
			for rName, rMap := range includedRegionRealms {
				logging.WithFields(logrus.Fields{
					"region": rName,
					"realms": len(rMap.values),
				}).Debug("Going over realms")

				// loading auctions
				loadedAuctions := func() chan loadAuctionsJob {
					if sta.resolver.config.UseGCloudStorage {
						return sta.resolver.store.loadRegionRealmMap(rMap)
					}

					return rMap.toRealms().loadAuctionsFromCacheDir(sta.resolver.config)
				}()
				loadedAuctionsResults := sta.liveAuctionsDatabases.load(loadedAuctions)
				for result := range loadedAuctionsResults {
					totalAuctions += len(result.stats.auctionIds)
					totalOwners += len(result.stats.ownerNames)
					totalRemovedAuctions += result.totalRemovedAuctions
					totalNewAuctions += result.totalNewAuctions
				}
				logging.WithFields(logrus.Fields{
					"region": rName,
					"realms": len(rMap.values),
				}).Debug("Finished loading auctions")
			}

			duration := time.Now().Sub(startTime)

			metric.ReportDuration(
				metric.AuctionsIntakeDuration,
				metric.DurationMetrics{
					Duration:       duration,
					TotalRealms:    totalRealms,
					IncludedRealms: includedRealmCount,
					ExcludedRealms: excludedRealmCount,
				},
				logrus.Fields{
					"total_auctions":          totalAuctions,
					"total_previous_auctions": totalPreviousAuctions,
					"total_new_auctions":      totalNewAuctions,
					"total_removed_auctions":  totalRemovedAuctions,
					"current_owner_count":     totalOwners,
					"current_item_count":      len(currentItemIds),
				},
			)
			logging.Info("Processed all realms")

			encodedAiRequest, err := json.Marshal(aiRequest)
			if err != nil {
				logging.WithField("error", err.Error()).Error("Failed to marshal auctions-intake-request")
			} else {
				sta.resolver.messenger.publish(subjects.PricelistsIntake, encodedAiRequest)
			}
		}
	}()

	// starting up a listener for auctions-intake
	err := sta.messenger.subscribe(subjects.AuctionsIntake, stop, func(natsMsg nats.Msg) {
		// resolving the request
		aiRequest, err := newAuctionsIntakeRequest(natsMsg.Data)
		if err != nil {
			logging.WithField("error", err.Error()).Error("Failed to parse auctions-intake-request")

			return
		}

		metric.ReportIntakeBufferSize(metric.LiveAuctionsIntake, len(in))
		logging.Info("Received auctions-intake-request")

		in <- aiRequest
	})
	if err != nil {
		return err
	}

	return nil
}
