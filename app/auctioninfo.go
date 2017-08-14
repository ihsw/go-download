package app

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ihsw/go-download/app/util"
)

const auctionInfoURLFormat = "https://%s.api.battle.net/wow/auction/data/%s"

func defaultGetAuctionInfoURL(regionHostname string, realmSlug realmSlug) string {
	return fmt.Sprintf(auctionInfoURLFormat, regionHostname, realmSlug)
}

type getAuctionInfoURLFunc func(string, realmSlug) string

func newAuctionInfoFromHTTP(rea realm, r resolver) (*auctionInfo, error) {
	body, err := util.Download(r.getAuctionInfoURL(rea.region.Hostname, rea.Slug))
	if err != nil {
		return nil, err
	}

	return newAuctionInfo(rea, body)
}

func newAuctionInfoFromFilepath(rea realm, relativeFilepath string) (*auctionInfo, error) {
	body, err := util.ReadFile(relativeFilepath)
	if err != nil {
		return nil, err
	}

	return newAuctionInfo(rea, body)
}

func newAuctionInfo(rea realm, body []byte) (*auctionInfo, error) {
	a := &auctionInfo{}
	if err := json.Unmarshal(body, a); err != nil {
		return nil, err
	}

	return a, nil
}

type auctionInfo struct {
	Files []auctionFile `json:"files"`
}

func (a auctionInfo) getFirstAuctions(r resolver) (*auctions, error) {
	if len(a.Files) == 0 {
		return nil, errors.New("cannot fetch first auctions with blank files")
	}

	return a.Files[0].getAuctions(r)
}

type auctionFile struct {
	URL          string `json:"url"`
	LastModified int64  `json:"lastModified"`
}

func (af auctionFile) getAuctions(r resolver) (*auctions, error) {
	return newAuctionsFromHTTP(af.URL, r)
}
