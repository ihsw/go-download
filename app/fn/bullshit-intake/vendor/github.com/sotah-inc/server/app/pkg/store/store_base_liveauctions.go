package store

import (
	"fmt"

	"cloud.google.com/go/storage"
	"github.com/sotah-inc/server/app/pkg/blizzard"
	"github.com/sotah-inc/server/app/pkg/sotah"
)

func NewLiveAuctionsBase(c Client, location string) LiveAuctionsBase {
	return LiveAuctionsBase{base{client: c, location: location}}
}

type LiveAuctionsBase struct {
	base
}

func (b LiveAuctionsBase) getBucketName() string {
	return "live-auctions"
}

func (b LiveAuctionsBase) GetFirmBucket() (*storage.BucketHandle, error) {
	return b.base.getFirmBucket(b.getBucketName())
}

func (b LiveAuctionsBase) GetBucket() *storage.BucketHandle {
	return b.base.getBucket(b.getBucketName())
}

func (b LiveAuctionsBase) resolveBucket() (*storage.BucketHandle, error) {
	return b.base.resolveBucket(b.getBucketName())
}

func (b LiveAuctionsBase) getObjectName(realm sotah.Realm) string {
	return fmt.Sprintf("%s-%s.json.gz", realm.Region.Name, realm.Slug)
}

func (b LiveAuctionsBase) GetObject(realm sotah.Realm, bkt *storage.BucketHandle) *storage.ObjectHandle {
	return b.base.getObject(b.getObjectName(realm), bkt)
}

func (b LiveAuctionsBase) GetFirmObject(realm sotah.Realm, bkt *storage.BucketHandle) (*storage.ObjectHandle, error) {
	return b.base.getFirmObject(b.getObjectName(realm), bkt)
}

func (b LiveAuctionsBase) Handle(aucs blizzard.Auctions, realm sotah.Realm, bkt *storage.BucketHandle) error {
	// encoding auctions in the appropriate format
	gzipEncodedBody, err := sotah.NewMiniAuctionListFromMiniAuctions(sotah.NewMiniAuctions(aucs)).EncodeForDatabase()
	if err != nil {
		return err
	}

	// writing it out to the gcloud object
	wc := b.GetObject(realm, bkt).NewWriter(b.client.Context)
	wc.ContentType = "application/json"
	wc.ContentEncoding = "gzip"
	if _, err := wc.Write(gzipEncodedBody); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}

	return nil
}
