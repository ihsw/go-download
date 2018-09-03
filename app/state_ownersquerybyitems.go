package main

import (
	"encoding/json"

	"github.com/ihsw/sotah-server/app/blizzard"

	"github.com/ihsw/sotah-server/app/codes"
	"github.com/ihsw/sotah-server/app/subjects"
	nats "github.com/nats-io/go-nats"
)

type ownerItemsOwnership struct {
	OwnedValue  int64 `json:"owned_value"`
	OwnedVolume int64 `json:"owned_volume"`
}

type ownersQueryResultByItems struct {
	Ownership   map[ownerName]ownerItemsOwnership `json:"ownership"`
	TotalValue  int64                             `json:"total_value"`
	TotalVolume int64                             `json:"total_volume"`
}

func newOwnersQueryRequestByItem(payload []byte) (ownersQueryRequestByItems, error) {
	request := &ownersQueryRequestByItems{}
	err := json.Unmarshal(payload, &request)
	if err != nil {
		return ownersQueryRequestByItems{}, err
	}

	return *request, nil
}

type ownersQueryRequestByItems struct {
	RegionName regionName         `json:"region_name"`
	RealmSlug  blizzard.RealmSlug `json:"realm_slug"`
	Items      []blizzard.ItemID  `json:"items"`
}

func (request ownersQueryRequestByItems) resolve(sta state) (miniAuctionList, requestError) {
	regionAuctions, ok := sta.auctions[request.RegionName]
	if !ok {
		return miniAuctionList{}, requestError{codes.NotFound, "Invalid region"}
	}

	realmAuctions, ok := regionAuctions[request.RealmSlug]
	if !ok {
		return miniAuctionList{}, requestError{codes.NotFound, "Invalid realm"}
	}

	return realmAuctions, requestError{codes.Ok, ""}
}

func (sta state) listenForOwnersQueryByItems(stop listenStopChan) error {
	err := sta.messenger.subscribe(subjects.OwnersQueryByItems, stop, func(natsMsg nats.Msg) {
		m := newMessage()

		// resolving the request
		request, err := newOwnersQueryRequestByItem(natsMsg.Data)
		if err != nil {
			m.Err = err.Error()
			m.Code = codes.MsgJSONParseError
			sta.messenger.replyTo(natsMsg, m)

			return
		}

		iMap := map[blizzard.ItemID]struct{}{}
		for _, ID := range request.Items {
			iMap[ID] = struct{}{}
		}

		// resolving the auctions
		aList, reErr := request.resolve(sta)
		if reErr.code != codes.Ok {
			m.Err = reErr.message
			m.Code = reErr.code
			sta.messenger.replyTo(natsMsg, m)

			return
		}

		// going over the auctions to gather results
		result := ownersQueryResultByItems{
			Ownership:   map[ownerName]ownerItemsOwnership{},
			TotalValue:  0,
			TotalVolume: 0,
		}
		for _, mAuction := range aList {
			if _, ok := iMap[mAuction.ItemID]; !ok {
				continue
			}

			aucListValue := mAuction.Buyout * mAuction.Quantity * int64(len(mAuction.AucList))
			aucListVolume := int64(len(mAuction.AucList)) * mAuction.Quantity

			result.TotalValue += aucListValue
			result.TotalVolume += aucListVolume

			if _, ok := result.Ownership[mAuction.Owner]; !ok {
				result.Ownership[mAuction.Owner] = ownerItemsOwnership{0, 0}
			}

			result.Ownership[mAuction.Owner] = ownerItemsOwnership{
				OwnedValue:  result.Ownership[mAuction.Owner].OwnedValue + aucListValue,
				OwnedVolume: result.Ownership[mAuction.Owner].OwnedVolume + aucListVolume,
			}
		}

		// marshalling for messenger
		encodedMessage, err := json.Marshal(result)
		if err != nil {
			m.Err = err.Error()
			m.Code = codes.GenericError
			sta.messenger.replyTo(natsMsg, m)

			return
		}

		// dumping it out
		m.Data = string(encodedMessage)
		sta.messenger.replyTo(natsMsg, m)
	})
	if err != nil {
		return err
	}

	return nil
}
