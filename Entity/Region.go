package Entity

import (
	"encoding/json"
	"github.com/ihsw/go-download/Cache"
	"github.com/ihsw/go-download/Config"
)

/*
	funcs
*/
func NewRegionFromConfig(configRegion Config.Region) Region {
	return Region{
		Name: configRegion.Name,
		Host: configRegion.Host,
	}
}

/*
	Region
*/
type Region struct {
	Id      int64
	Name    string
	Host    string
	Locales map[int64]Locale
}

func (self Region) Marshal() (string, error) {
	var (
		s string
	)

	v := map[string]interface{}{
		"0": self.Id,
		"1": self.Name,
		"2": self.Host,
	}
	b, err := json.Marshal(v)
	if err != nil {
		return s, err
	}

	return string(b), nil
}

/*
	RegionManager
*/
type RegionManager struct {
	Client Cache.Client
}

func (self RegionManager) Persist(region Region) (Region, error) {
	var (
		err error
		s   string
	)
	main := self.Client.Main

	if region.Id == 0 {
		region.Id, err = main.Incr("region_id")
		if err != nil {
			return region, err
		}
	}

	s, err = region.Marshal()
	if err != nil {
		return region, err
	}

	bucketKey, subKey := Cache.GetBucketKey(region.Id, "region")
	err = main.HSet(bucketKey, subKey, s)
	if err != nil {
		return region, err
	}

	return region, nil
}

func (self RegionManager) Unmarshal(v map[string]interface{}) Region {
	return Region{
		Id:   int64(v["0"].(float64)),
		Name: v["1"].(string),
		Host: v["2"].(string),
	}
}

func (self RegionManager) FindOneById(id int64) (region Region, err error) {
	var s string
	bucketKey, subKey := Cache.GetBucketKey(id, "region")
	s, err = self.Client.Main.HGet(bucketKey, subKey)
	if err != nil {
		return
	}

	b := []byte(s)
	v := map[string]interface{}{}
	err = json.Unmarshal(b, &v)
	if err != nil {
		return
	}

	region = self.Unmarshal(v)
	return
}
