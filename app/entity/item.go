package entity

import (
	"encoding/json"
	"strconv"

	"github.com/ihsw/go-download/app/cache"
)

/*
	Item
*/
type Item struct {
	Id      int64
	BlizzId int64
}

func (self Item) marshal() (string, error) {
	itemJson := ItemJson{
		Id:      self.Id,
		BlizzId: self.BlizzId,
	}

	return itemJson.marshal()
}

/*
	ItemJson
*/
type ItemJson struct {
	Id      int64 `json:"0"`
	BlizzId int64 `json:"1"`
}

func (self ItemJson) marshal() (string, error) {
	b, err := json.Marshal(self)
	return string(b), err
}

/*
	ItemManager
*/
type ItemManager struct {
	Client cache.Client
}

func (self ItemManager) Namespace() string { return "item" }

func (self ItemManager) PersistAll(items []Item) (err error) {
	m := self.Client.Main

	// ids
	var ids []int64
	if ids, err = m.IncrAll("item_id", len(items)); err != nil {
		return
	}
	for i, id := range ids {
		items[i].Id = id
	}

	// data
	values := make([]cache.PersistValue, len(items))
	for i, item := range items {
		bucketKey, subKey := cache.GetBucketKey(item.Id, self.Namespace())

		var s string
		if s, err = item.marshal(); err != nil {
			return
		}

		values[i] = cache.PersistValue{
			BucketKey: bucketKey,
			SubKey:    subKey,
			Value:     s,
		}
	}
	if err = m.PersistAll(values); err != nil {
		return
	}

	// etc
	newIds := make([]string, len(items))
	newBlizzIds := make([]string, len(items))
	for i, item := range items {
		newIds[i] = strconv.FormatInt(item.Id, 10)
		newBlizzIds[i] = strconv.FormatInt(item.BlizzId, 10)
	}
	if err = m.RPushAll("item:ids", newIds); err != nil {
		return
	}
	if err = m.SAddAll("item:blizz_ids", newBlizzIds); err != nil {
		return
	}

	return nil
}

func (self ItemManager) unmarshal(v string) (item Item, err error) {
	if v == "" {
		return
	}

	// json
	var itemJson ItemJson
	b := []byte(v)
	err = json.Unmarshal(b, &itemJson)
	if err != nil {
		return
	}

	// initial
	item = Item{
		Id:      itemJson.Id,
		BlizzId: itemJson.BlizzId,
	}

	return item, nil
}

func (self ItemManager) unmarshalAll(values []string) (items []Item, err error) {
	items = make([]Item, len(values))
	for i, v := range values {
		items[i], err = self.unmarshal(v)
		if err != nil {
			return
		}
	}
	return
}

func (self ItemManager) FindAll() (items []Item, err error) {
	m := self.Client.Main

	// fetching ids
	ids, err := m.FetchIds("item:ids", 0, -1)
	if err != nil {
		return
	}

	// fetching the values
	var values []string
	values, err = m.FetchFromIds(self, ids)
	if err != nil {
		return
	}

	return self.unmarshalAll(values)
}

func (self ItemManager) GetBlizzIds() (blizzIds []int64, err error) {
	var values []string
	if values, err = self.Client.Main.SMembers("item:blizz_ids"); err != nil {
		return
	}

	blizzIds = make([]int64, len(values))
	for i, v := range values {
		var blizzId int
		if blizzId, err = strconv.Atoi(v); err != nil {
			return
		}
		blizzIds[i] = int64(blizzId)
	}
	return
}
