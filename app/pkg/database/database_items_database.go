package database

import (
	"encoding/json"
	"strconv"

	"github.com/boltdb/bolt"
	"github.com/sotah-inc/server/app/pkg/blizzard"
	"github.com/sotah-inc/server/app/pkg/logging"
	"github.com/sotah-inc/server/app/pkg/sotah"
	"github.com/sotah-inc/server/app/pkg/util"
)

func NewItemsDatabase(dbDir string) (ItemsDatabase, error) {
	dbFilepath, err := itemsDatabasePath(dbDir)
	if err != nil {
		return ItemsDatabase{}, err
	}

	logging.WithField("filepath", dbFilepath).Info("Initializing items database")

	db, err := bolt.Open(dbFilepath, 0600, nil)
	if err != nil {
		return ItemsDatabase{}, err
	}

	return ItemsDatabase{db}, nil
}

type ItemsDatabase struct {
	db *bolt.DB
}

// gathering items
func (idBase ItemsDatabase) GetItems() (sotah.ItemsMap, error) {
	out := sotah.ItemsMap{}

	err := idBase.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(databaseItemsBucketName())
		if bkt == nil {
			return nil
		}

		err := bkt.ForEach(func(k, v []byte) error {
			parsedId, err := strconv.Atoi(string(k)[len("item-"):])
			if err != nil {
				return err
			}
			itemId := blizzard.ItemID(parsedId)

			gzipDecoded, err := util.GzipDecode(v)
			if err != nil {
				return err
			}

			item, err := sotah.NewItem(gzipDecoded)
			if err != nil {
				return err
			}

			out[itemId] = item

			return nil
		})
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return sotah.ItemsMap{}, err
	}

	return out, nil
}

func (idBase ItemsDatabase) FindItems(itemIds []blizzard.ItemID) (sotah.ItemsMap, error) {
	out := sotah.ItemsMap{}
	err := idBase.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(databaseItemsBucketName())
		if bkt == nil {
			return nil
		}

		for _, id := range itemIds {
			value := bkt.Get(itemsKeyName(id))
			if value == nil {
				continue
			}

			gzipDecoded, err := util.GzipDecode(value)
			if err != nil {
				return err
			}

			item, err := sotah.NewItem(gzipDecoded)
			if err != nil {
				return err
			}

			out[id] = item
		}

		return nil
	})
	if err != nil {
		return sotah.ItemsMap{}, err
	}

	return out, nil
}

// persisting
func (idBase ItemsDatabase) PersistItems(iMap sotah.ItemsMap) error {
	logging.WithField("items", len(iMap)).Debug("Persisting items")

	err := idBase.db.Batch(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists(databaseItemsBucketName())
		if err != nil {
			return err
		}

		for id, item := range iMap {
			jsonEncoded, err := json.Marshal(item)
			if err != nil {
				return err
			}

			gzipEncoded, err := util.GzipEncode(jsonEncoded)
			if err != nil {
				return err
			}

			if err := bkt.Put(itemsKeyName(id), gzipEncoded); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

type PersistEncodedItemsInJob struct {
	Id              blizzard.ItemID
	GzipEncodedData []byte
}

func (idBase ItemsDatabase) PersistEncodedItems(in chan PersistEncodedItemsInJob) error {
	logging.Info("Persisting encoded items")

	err := idBase.db.Batch(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists(databaseItemsBucketName())
		if err != nil {
			return err
		}

		for job := range in {
			if err := bkt.Put(itemsKeyName(job.Id), job.GzipEncodedData); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (idBase ItemsDatabase) FilterInItemsToSync(ids blizzard.ItemIds) (blizzard.ItemIds, error) {
	exists := map[blizzard.ItemID]bool{}
	for _, id := range ids {
		exists[id] = false
	}

	err := idBase.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(databaseItemsBucketName())
		if bkt == nil {
			return nil
		}

		for _, id := range ids {
			value := bkt.Get(itemsKeyName(id))
			if value == nil {
				continue
			}

			exists[id] = true
		}

		return nil
	})
	if err != nil {
		return blizzard.ItemIds{}, err
	}

	out := blizzard.ItemIds{}
	for id, exists := range exists {
		if exists {
			continue
		}

		out = append(out, id)
	}

	return out, nil
}
