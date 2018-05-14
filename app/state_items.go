package main

import (
	"encoding/json"
	"errors"

	"github.com/ihsw/sotah-server/app/codes"
	"github.com/ihsw/sotah-server/app/subjects"
	nats "github.com/nats-io/go-nats"
)

func newItemsRequest(payload []byte) (*itemsRequest, error) {
	request := &itemsRequest{}
	err := json.Unmarshal(payload, &request)
	if err != nil {
		return nil, err
	}

	return request, nil
}

type itemsRequest struct {
	Query string `json:"query"`
}

func (request itemsRequest) resolve(sta state) (map[itemID]*item, error) {
	if sta.items == nil {
		return nil, errors.New("Items were nil")
	}

	return sta.items, nil
}

func (sta state) listenForItems(stop chan interface{}) error {
	err := sta.messenger.subscribe(subjects.Items, stop, func(natsMsg *nats.Msg) {
		m := newMessage()

		// resolving the request
		request, err := newItemsRequest(natsMsg.Data)
		if err != nil {
			m.Err = err.Error()
			m.Code = codes.MsgJSONParseError
			sta.messenger.replyTo(natsMsg, m)

			return
		}

		// resolving the list of items
		stateItems, err := request.resolve(sta)
		if err != nil {
			m.Err = err.Error()
			m.Code = codes.GenericError
			sta.messenger.replyTo(natsMsg, m)

			return
		}

		// marshalling for messenger
		encodedMessage, err := json.Marshal(stateItems)
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
