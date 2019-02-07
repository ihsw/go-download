package state

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/sotah-inc/server/app/pkg/sotah"

	nats "github.com/nats-io/go-nats"
	"github.com/sotah-inc/server/app/pkg/blizzard"
	"github.com/sotah-inc/server/app/pkg/database"
	"github.com/sotah-inc/server/app/pkg/logging"
	"github.com/sotah-inc/server/app/pkg/messenger"
	"github.com/sotah-inc/server/app/pkg/messenger/codes"
	"github.com/sotah-inc/server/app/pkg/messenger/subjects"
	"github.com/sotah-inc/server/app/pkg/util"
)

type priceListHistoryResponse struct {
	History sotah.ItemPriceHistories `json:"history"`
}

func (plhResponse priceListHistoryResponse) encodeForMessage() (string, error) {
	jsonEncodedResponse, err := json.Marshal(plhResponse)
	if err != nil {
		return "", err
	}

	gzipEncodedResponse, err := util.GzipEncode(jsonEncodedResponse)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(gzipEncodedResponse), nil
}

func newPriceListHistoryRequest(payload []byte) (priceListHistoryRequest, error) {
	plhRequest := &priceListHistoryRequest{}
	err := json.Unmarshal(payload, &plhRequest)
	if err != nil {
		return priceListHistoryRequest{}, err
	}

	return *plhRequest, nil
}

type priceListHistoryRequest struct {
	RegionName  blizzard.RegionName `json:"region_name"`
	RealmSlug   blizzard.RealmSlug  `json:"realm_slug"`
	ItemIds     []blizzard.ItemID   `json:"item_ids"`
	LowerBounds int64               `json:"lower_bounds"`
	UpperBounds int64               `json:"upper_bounds"`
}

func (plhRequest priceListHistoryRequest) resolve(sta PricelistHistoriesState) (sotah.Realm, database.PricelistHistoryDatabaseShards, requestError) {
	regionStatuses, ok := sta.Statuses[plhRequest.RegionName]
	if !ok {
		return sotah.Realm{},
			database.PricelistHistoryDatabaseShards{},
			requestError{codes.NotFound, "Invalid region (statuses)"}
	}
	rea := func() *sotah.Realm {
		for _, regionRealm := range regionStatuses.Realms {
			if regionRealm.Slug == plhRequest.RealmSlug {
				return &regionRealm
			}
		}

		return nil
	}()
	if rea == nil {
		return sotah.Realm{},
			database.PricelistHistoryDatabaseShards{},
			requestError{codes.NotFound, "Invalid realm (statuses)"}
	}

	phdShards, reErr := func() (database.PricelistHistoryDatabaseShards, requestError) {
		regionShards, ok := sta.IO.Databases.PricelistHistoryDatabases.Databases[plhRequest.RegionName]
		if !ok {
			return database.PricelistHistoryDatabaseShards{}, requestError{codes.NotFound, "Invalid region (pricelist-history databases)"}
		}

		realmShards, ok := regionShards[plhRequest.RealmSlug]
		if !ok {
			return database.PricelistHistoryDatabaseShards{}, requestError{codes.NotFound, "Invalid realm (pricelist-histories)"}
		}

		return realmShards, requestError{codes.Ok, ""}
	}()

	return *rea, phdShards, reErr
}

func (sta PricelistHistoriesState) ListenForPriceListHistory(stop messenger.ListenStopChan) error {
	err := sta.IO.Messenger.Subscribe(subjects.PriceListHistory, stop, func(natsMsg nats.Msg) {
		m := messenger.NewMessage()

		// resolving the request
		plhRequest, err := newPriceListHistoryRequest(natsMsg.Data)
		if err != nil {
			m.Err = err.Error()
			m.Code = codes.MsgJSONParseError
			sta.IO.Messenger.ReplyTo(natsMsg, m)

			return
		}

		// resolving the database from the request
		rea, phdShards, reErr := plhRequest.resolve(sta)
		if reErr.code != codes.Ok {
			m.Err = reErr.message
			m.Code = reErr.code
			sta.IO.Messenger.ReplyTo(natsMsg, m)

			return
		}

		logging.WithField("database-shards", len(phdShards)).Info("Querying database shards")

		// gathering up pricelist history
		plhResponse := priceListHistoryResponse{History: sotah.ItemPriceHistories{}}
		for _, ID := range plhRequest.ItemIds {
			plHistory, err := phdShards.GetPriceHistory(
				rea,
				ID,
				time.Unix(plhRequest.LowerBounds, 0),
				time.Unix(plhRequest.UpperBounds, 0),
			)
			if err != nil {
				m.Err = err.Error()
				m.Code = codes.GenericError
				sta.IO.Messenger.ReplyTo(natsMsg, m)

				return
			}

			plhResponse.History[ID] = plHistory
		}

		// encoding the message for the response
		data, err := plhResponse.encodeForMessage()
		if err != nil {
			m.Err = err.Error()
			m.Code = codes.MsgJSONParseError
			sta.IO.Messenger.ReplyTo(natsMsg, m)

			return
		}

		m.Data = data
		sta.IO.Messenger.ReplyTo(natsMsg, m)
	})
	if err != nil {
		return err
	}

	return nil
}
