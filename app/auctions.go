package app

import (
	"encoding/json"

	"github.com/ihsw/go-download/app/util"
)

func defaultGetAuctionsURL(url string) string {
	return url
}

type getAuctionsURLFunc func(url string) string

func newAuctions(url string, r resolver) (*auctions, error) {
	body, err := util.Download(r.getAuctionsURL(url))
	if err != nil {
		return nil, err
	}

	a := &auctions{}
	if err := json.Unmarshal(body, a); err != nil {
		return nil, err
	}

	return a, nil
}

type auctions struct {
	Realms   []auctionRealm `json:"realms"`
	Auctions []auction      `json:"auctions"`
}

type auctionRealm struct {
	Name string    `json:"name"`
	Slug realmSlug `json:"slug"`
}

type auction struct {
	Auc        int64  `json:"auc"`
	Item       int64  `json:"item"`
	Owner      string `json:"owner"`
	PwnerRealm string `json:"ownerRealm"`
	Bid        int64  `json:"bid"`
	Buyout     int64  `json:"buyout"`
	Quantity   int64  `json:"quantity"`
	TimeLeft   string `json:"timeLeft"`
	Rand       int64  `json:"rand"`
	Seed       int64  `json:"seed"`
	Context    int64  `json:"context"`
}
