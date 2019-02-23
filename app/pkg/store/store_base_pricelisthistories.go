package store

import (
	"fmt"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"github.com/sotah-inc/server/app/pkg/blizzard"
	"github.com/sotah-inc/server/app/pkg/logging"
	"github.com/sotah-inc/server/app/pkg/sotah"
)

func NewPricelistHistoriesBase(c Client) PricelistHistoriesBase {
	return PricelistHistoriesBase{base{client: c}}
}

type PricelistHistoriesBase struct {
	base
}

func (b PricelistHistoriesBase) getBucketName(rea sotah.Realm) string {
	return fmt.Sprintf("pricelist-histories_%s_%s", rea.Region.Name, rea.Slug)
}

func (b PricelistHistoriesBase) GetBucket(rea sotah.Realm) *storage.BucketHandle {
	return b.base.getBucket(b.getBucketName(rea))
}

func (b PricelistHistoriesBase) resolveBucket(rea sotah.Realm) (*storage.BucketHandle, error) {
	return b.base.resolveBucket(b.getBucketName(rea))
}

func (b PricelistHistoriesBase) getObjectName(targetTime time.Time) string {
	return fmt.Sprintf("%d.txt.gz", targetTime.Unix())
}

func (b PricelistHistoriesBase) getObject(targetTime time.Time, bkt *storage.BucketHandle) *storage.ObjectHandle {
	return b.base.getObject(b.getObjectName(targetTime), bkt)
}

func (b PricelistHistoriesBase) Handle(aucs blizzard.Auctions, targetTime time.Time, rea sotah.Realm) (sotah.UnixTimestamp, error) {
	normalizedTargetDate := sotah.NormalizeTargetDate(targetTime)

	logging.WithFields(logrus.Fields{
		"region":                 rea.Region.Name,
		"realm":                  rea.Slug,
		"target-time":            targetTime.Unix(),
		"normalized-target-date": normalizedTargetDate.Unix(),
	}).Info("Processing")

	// resolving unix-timestamp of target-time
	targetTimestamp := sotah.UnixTimestamp(targetTime.Unix())

	// gathering the bucket
	bkt, err := b.resolveBucket(rea)
	if err != nil {
		return 0, err
	}

	// gathering an object
	obj := b.getObject(normalizedTargetDate, bkt)

	// resolving item-price-histories
	ipHistories, err := func() (sotah.ItemPriceHistories, error) {
		exists, err := b.objectExists(obj)
		if err != nil {
			return sotah.ItemPriceHistories{}, err
		}

		if !exists {
			return sotah.ItemPriceHistories{}, nil
		}

		reader, err := obj.NewReader(b.client.Context)
		if err != nil {
			return sotah.ItemPriceHistories{}, err
		}
		defer reader.Close()

		return sotah.NewItemPriceHistoriesFromMinimized(reader)
	}()
	if err != nil {
		return 0, err
	}

	// gathering new item-prices from the input
	iPrices := sotah.NewItemPrices(sotah.NewMiniAuctionListFromMiniAuctions(sotah.NewMiniAuctions(aucs)))

	// merging item-prices into the item-price-histories
	for itemId, prices := range iPrices {
		pHistory := func() sotah.PriceHistory {
			result, ok := ipHistories[itemId]
			if !ok {
				return sotah.PriceHistory{}
			}

			return result
		}()
		pHistory[targetTimestamp] = prices

		ipHistories[itemId] = pHistory
	}

	// encoding the item-price-histories for persistence
	gzipEncodedBody, err := ipHistories.EncodeForPersistence()
	if err != nil {
		return 0, err
	}

	// writing it out to the gcloud object
	wc := obj.NewWriter(b.client.Context)
	wc.ContentType = "text/plain"
	wc.ContentEncoding = "gzip"
	if _, err := wc.Write(gzipEncodedBody); err != nil {
		return 0, err
	}

	return sotah.UnixTimestamp(normalizedTargetDate.Unix()), wc.Close()
}
