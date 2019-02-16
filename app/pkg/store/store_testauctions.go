package store

import (
	"encoding/json"
	"fmt"

	"github.com/sotah-inc/server/app/pkg/util"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"github.com/sotah-inc/server/app/pkg/blizzard"
	"github.com/sotah-inc/server/app/pkg/logging"
	"github.com/sotah-inc/server/app/pkg/sotah"
)

func (sto Store) getTestAuctionsBucketName() string {
	return "test-auctions"
}

func (sto Store) GetTestAuctionsBucket() *storage.BucketHandle {
	return sto.client.Bucket(sto.getTestAuctionsBucketName())
}

func (sto Store) createTestAuctionsBucket() (*storage.BucketHandle, error) {
	bkt := sto.GetTestAuctionsBucket()
	err := bkt.Create(sto.Context, sto.projectID, &storage.BucketAttrs{
		StorageClass: "REGIONAL",
		Location:     "us-east1",
	})
	if err != nil {
		return nil, err
	}

	return bkt, nil
}

func (sto Store) TestAuctionsBucketExists() (bool, error) {
	_, err := sto.GetTestAuctionsBucket().Attrs(sto.Context)
	if err != nil {
		if err != storage.ErrBucketNotExist {
			return false, err
		}

		return false, nil
	}

	return true, nil
}

func (sto Store) resolveTestAuctionsBucket() (*storage.BucketHandle, error) {
	exists, err := sto.TestAuctionsBucketExists()
	if err != nil {
		return nil, err
	}

	if !exists {
		return sto.createTestAuctionsBucket()
	}

	return sto.GetTestAuctionsBucket(), nil
}

func (sto Store) GetTestAuctionsObjectName(rea sotah.Realm) string {
	return fmt.Sprintf("%s-%s.json.gz", rea.Region.Name, rea.Slug)
}

func (sto Store) getTestAuctionsObject(bkt *storage.BucketHandle, rea sotah.Realm) *storage.ObjectHandle {
	return bkt.Object(sto.GetTestAuctionsObjectName(rea))
}

func (sto Store) testAuctionsObjectExists(bkt *storage.BucketHandle, rea sotah.Realm) (bool, error) {
	_, err := sto.getTestAuctionsObject(bkt, rea).Attrs(sto.Context)
	if err != nil {
		if err != storage.ErrObjectNotExist {
			return false, err
		}

		return false, nil
	}

	return true, nil
}

func (sto Store) WriteTestAuctions(rea sotah.Realm, gzipEncodedBody []byte) error {
	bkt, err := sto.resolveTestAuctionsBucket()
	if err != nil {
		return err
	}

	logging.WithFields(logrus.Fields{
		"region": rea.Region.Name,
		"realm":  rea.Slug,
		"length": len(gzipEncodedBody),
	}).Debug("Writing auctions to gcloud storage")

	wc := bkt.Object(sto.GetTestAuctionsObjectName(rea)).NewWriter(sto.Context)
	wc.ContentType = "application/json"
	wc.ContentEncoding = "gzip"

	if _, err := wc.Write(gzipEncodedBody); err != nil {
		return err
	}

	return wc.Close()
}

func (sto Store) LoadTestAuctions(in chan LoadAuctionsInJob) chan LoadAuctionsOutJob {
	out := make(chan LoadAuctionsOutJob)

	// spinning up the workers for fetching Auctions
	worker := func() {
		for inJob := range in {
			jsonEncodedData, err := json.Marshal(inJob.Auctions)
			if err != nil {
				out <- LoadAuctionsOutJob{
					Err:        err,
					Realm:      inJob.Realm,
					TargetTime: inJob.TargetTime,
					ItemIds:    []blizzard.ItemID{},
				}

				continue
			}

			gzipEncodedData, err := util.GzipEncode(jsonEncodedData)
			if err != nil {
				out <- LoadAuctionsOutJob{
					Err:        err,
					Realm:      inJob.Realm,
					TargetTime: inJob.TargetTime,
					ItemIds:    []blizzard.ItemID{},
				}

				continue
			}

			if err := sto.WriteTestAuctions(inJob.Realm, gzipEncodedData); err != nil {
				out <- LoadAuctionsOutJob{
					Err:        err,
					Realm:      inJob.Realm,
					TargetTime: inJob.TargetTime,
					ItemIds:    []blizzard.ItemID{},
				}

				continue
			}

			outItemIds := []blizzard.ItemID{}
			for _, auc := range inJob.Auctions.Auctions {
				outItemIds = append(outItemIds, auc.Item)
			}

			out <- LoadAuctionsOutJob{
				Err:        nil,
				Realm:      inJob.Realm,
				TargetTime: inJob.TargetTime,
				ItemIds:    outItemIds,
			}
		}
	}
	postWork := func() {
		close(out)
	}
	util.Work(4, worker, postWork)

	return out
}

type GetTestAuctionsOutJob struct {
	Err      error
	Realm    sotah.Realm
	Auctions blizzard.Auctions
}

func (job GetTestAuctionsOutJob) ToLogrusFields() logrus.Fields {
	return logrus.Fields{
		"error": job.Err.Error(),
		"realm": job.Realm.Slug,
	}
}

func (sto Store) GetTestAuctionsFromRealms(realmMaps sotah.RegionRealmMap) chan GetTestAuctionsOutJob {
	in := make(chan sotah.Realm)
	out := make(chan GetTestAuctionsOutJob)

	// spinning up the workers for fetching Auctions
	worker := func() {
		for realm := range in {
			aucs, err := sto.GetTestAuctions(realm)
			if err != nil {
				out <- GetTestAuctionsOutJob{
					Err:      err,
					Realm:    realm,
					Auctions: blizzard.Auctions{},
				}

				continue
			}

			out <- GetTestAuctionsOutJob{
				Err:      nil,
				Realm:    realm,
				Auctions: aucs,
			}
		}
	}
	postWork := func() {
		close(out)
	}
	util.Work(4, worker, postWork)

	// queueing up the Realms
	go func() {
		for _, realmMap := range realmMaps {
			for _, realm := range realmMap {
				logging.WithFields(logrus.Fields{
					"region": realm.Region.Name,
					"realm":  realm.Slug,
				}).Debug("Queueing up realm for loading")
				in <- realm
			}
		}

		close(in)
	}()

	return out
}

func (sto Store) GetTestAuctions(realm sotah.Realm) (blizzard.Auctions, error) {
	bkt := sto.GetTestAuctionsBucket()

	return sto.NewAuctions(sto.getTestAuctionsObject(bkt, realm))
}
