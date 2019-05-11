package hell

import (
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/sotah-inc/server/app/pkg/blizzard"
	"github.com/sotah-inc/server/app/pkg/hell/collections"
	"github.com/sotah-inc/server/app/pkg/sotah"
	"github.com/sotah-inc/server/app/pkg/sotah/gameversions"
	"github.com/sotah-inc/server/app/pkg/util"
)

func (c Client) GetRealm(realmRef *firestore.DocumentRef) (Realm, error) {
	docsnap, err := realmRef.Get(c.Context)
	if err != nil {
		return Realm{}, err
	}

	var realmData Realm
	if err := docsnap.DataTo(&realmData); err != nil {
		return Realm{}, err
	}

	return realmData, nil
}

type WriteRealmsJob struct {
	Err     error
	Payload WriteRealmsPayload
}

func NewWriteRealmsPayloads(realms sotah.Realms) WriteRealmsPayloads {
	out := WriteRealmsPayloads{}
	for _, realm := range realms {
		out = append(out, NewWriteRealmsPayload(realm))
	}

	return out
}

type WriteRealmsPayloads []WriteRealmsPayload

func NewWriteRealmsPayload(realm sotah.Realm) WriteRealmsPayload {
	return WriteRealmsPayload{
		RegionName:             realm.Region.Name,
		RealmSlug:              realm.Slug,
		RealmModificationDates: realm.RealmModificationDates,
	}
}

type WriteRealmsPayload struct {
	RegionName             blizzard.RegionName
	RealmSlug              blizzard.RealmSlug
	RealmModificationDates sotah.RealmModificationDates
}

func (c Client) WriteRealms(payloads []WriteRealmsPayload, version gameversions.GameVersion) error {
	// spawning workers
	in := make(chan WriteRealmsPayload)
	out := make(chan WriteRealmsJob)
	worker := func() {
		for payload := range in {
			realmRef, err := c.FirmDocument(fmt.Sprintf(
				"%s/%s/%s/%s/%s/%s",
				collections.Games,
				version,
				collections.Regions,
				payload.RegionName,
				collections.Realms,
				payload.RealmSlug,
			))
			if err != nil {
				out <- WriteRealmsJob{
					Err:     err,
					Payload: payload,
				}

				continue
			}

			if _, err := realmRef.Set(c.Context, NewRealm(payload)); err != nil {
				out <- WriteRealmsJob{
					Err:     err,
					Payload: payload,
				}

				continue
			}

			out <- WriteRealmsJob{
				Err:     nil,
				Payload: payload,
			}
		}
	}
	postWork := func() {
		close(out)
	}
	util.Work(8, worker, postWork)

	// spinning it up
	go func() {
		for _, payload := range payloads {
			in <- payload
		}

		close(in)
	}()

	// waiting for results to drain out
	for job := range out {
		if job.Err != nil {
			return job.Err
		}
	}

	return nil
}

type GetRealmsJob struct {
	Err        error
	RegionName blizzard.RegionName
	Realm      Realm
}

type GetRealmsPayload struct {
	RegionName blizzard.RegionName
	RealmSlug  blizzard.RealmSlug
}

func (c Client) GetRealms(regionRealmSlugs map[blizzard.RegionName][]blizzard.RealmSlug, version gameversions.GameVersion) (RegionRealms, error) {
	// spawning workers
	in := make(chan GetRealmsPayload)
	out := make(chan GetRealmsJob)
	worker := func() {
		for payload := range in {
			realmRef, err := c.FirmDocument(fmt.Sprintf(
				"%s/%s/%s/%s/%s/%s",
				collections.Games,
				version,
				collections.Regions,
				payload.RegionName,
				collections.Realms,
				payload.RealmSlug,
			))
			if err != nil {
				out <- GetRealmsJob{
					Err:        err,
					RegionName: blizzard.RegionName(""),
					Realm:      Realm{},
				}

				continue
			}

			docsnap, err := realmRef.Get(c.Context)
			if err != nil {
				out <- GetRealmsJob{
					Err:        err,
					RegionName: blizzard.RegionName(""),
					Realm:      Realm{},
				}

				continue
			}

			var realm Realm
			if err := docsnap.DataTo(&realm); err != nil {
				out <- GetRealmsJob{
					Err:        err,
					RegionName: blizzard.RegionName(""),
					Realm:      Realm{},
				}

				continue
			}

			out <- GetRealmsJob{
				Err:        nil,
				RegionName: payload.RegionName,
				Realm:      realm,
			}
		}
	}
	postWork := func() {
		close(out)
	}
	util.Work(8, worker, postWork)

	// spinning it up
	go func() {
		for regionName, realmSlugs := range regionRealmSlugs {
			for _, realmSlug := range realmSlugs {
				in <- GetRealmsPayload{
					RegionName: regionName,
					RealmSlug:  realmSlug,
				}
			}
		}

		close(in)
	}()

	// waiting for results to drain out
	regionRealms := RegionRealms{}
	for job := range out {
		if job.Err != nil {
			return RegionRealms{}, job.Err
		}

		next := func() Realms {
			result, ok := regionRealms[job.RegionName]
			if !ok {
				return Realms{}
			}

			return result
		}()
		next = append(next, job.Realm)

		regionRealms[job.RegionName] = next
	}

	return regionRealms, nil
}

func NewRealm(payload WriteRealmsPayload) Realm {
	return Realm{
		Slug:                       string(payload.RealmSlug),
		Downloaded:                 int(payload.RealmModificationDates.Downloaded),
		LiveAuctionsReceived:       int(payload.RealmModificationDates.LiveAuctionsReceived),
		PricelistHistoriesReceived: int(payload.RealmModificationDates.PricelistHistoriesReceived),
	}
}

type Realm struct {
	Slug                       string `firestore:"slug"`
	Downloaded                 int    `firestore:"downloaded"`
	LiveAuctionsReceived       int    `firestore:"live_auctions_received"`
	PricelistHistoriesReceived int    `firestore:"pricelist_histories_received"`
}

type Realms []Realm

type RegionRealms map[blizzard.RegionName]Realms
