package main

import (
	"fmt"
	"github.com/ihsw/go-download/Cache"
	"github.com/ihsw/go-download/Entity"
	"github.com/ihsw/go-download/Misc"
	"github.com/ihsw/go-download/Util"
	"github.com/ihsw/go-download/Work"
	"os"
	"runtime"
	"time"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	output := Util.Output{
		StartTime: time.Now(),
	}
	output.Write("Starting...")

	var (
		err          error
		cacheClient  Cache.Client
		regions      []Entity.Region
		regionRealms map[int64][]Entity.Realm
	)
	debug := true

	/*
		reading the config
	*/
	// gathering a cache client and regions after reading the config
	output.Write("Initializing the cache-client and regions...")
	cacheClient, regions, err = Misc.GetCacheClientAndRegions(os.Args, true)
	if err != nil {
		output.Write(fmt.Sprintf("Misc.GetCacheClientAndRegions() fail: %s", err.Error()))
		return
	}

	/*
		gathering the realms for each region
	*/
	output.Write("Fetching realms for each region...")
	regionRealms, err = Misc.GetRealms(cacheClient, regions)
	if err != nil {
		output.Write(fmt.Sprintf("Misc.GetRealms() fail: %s", err.Error()))
		return
	}

	/*
		removing realms that aren't queryable
	*/
	totalRealms := 0
	for _, region := range regions {
		if !region.Queryable {
			delete(regionRealms, region.Id)
			continue
		}
		if debug {
			totalRealms += 1
		} else {
			totalRealms += len(regionRealms[region.Id])
		}
	}

	regionMap := map[int64]int64{}
	for i, region := range regions {
		regionMap[region.Id] = int64(i)
	}

	/*
		making channels and spawning workers
	*/
	// misc
	downloadIn := make(chan Entity.Realm, totalRealms)
	itemizeIn := make(chan Work.DownloadResult, totalRealms)
	itemizeOut := make(chan Work.ItemizeResult, totalRealms)

	// spawning some download workers
	output.Write("Spawning some download workers...")
	downloadWorkerCount := 4
	for j := 0; j < downloadWorkerCount; j++ {
		go func(in chan Entity.Realm, out chan Work.DownloadResult, output Util.Output) {
			for {
				Work.DownloadRealm(<-in, out, output)
			}
		}(downloadIn, itemizeIn, output)
	}

	// spawning an itemize worker
	go func(in chan Work.DownloadResult, out chan Work.ItemizeResult) {
		for {
			Work.ItemizeRealm(<-in, out)
		}
	}(itemizeIn, itemizeOut)

	/*
		queueing up the realms
	*/
	// formatting the realms to be evenly distributed
	largestRegion := 0
	for _, realms := range regionRealms {
		if len(realms) > largestRegion {
			largestRegion = len(realms)
		}
	}
	formattedRealms := make([]map[int64]Entity.Realm, largestRegion)
	for regionId, realms := range regionRealms {
		for i, realm := range realms {
			if formattedRealms[int64(i)] == nil {
				formattedRealms[int64(i)] = map[int64]Entity.Realm{}
			}
			formattedRealms[int64(i)][regionId] = realm
		}
	}

	// pushing the realms into the start of the queue
	output.Write("Queueing up the realms for checking...")
	for _, realms := range formattedRealms {
		for _, realm := range realms {
			downloadIn <- realm
		}

		if debug {
			break
		}
	}

	/*
		debugging
	*/
	output.Write(fmt.Sprintf("Gathering %d results for debugging...", totalRealms))
	results := make([]Work.ItemizeResult, totalRealms)
	for i := 0; i < totalRealms; i++ {
		results[i] = <-itemizeOut
	}

	output.Write(fmt.Sprintf("Going over %d results for debugging...", len(results)))
	totalAuctionCount := 0
	for _, result := range results {
		realm := result.Realm
		if result.Error != nil {
			output.Write(fmt.Sprintf("Itemize %s fail: %s", realm.Dump(), result.Error.Error()))
			continue
		}

		totalAuctionCount += result.AuctionCount
		output.Write(fmt.Sprintf("%s has %d auctions...", realm.Dump(), result.AuctionCount))
	}
	output.Write(fmt.Sprintf("%d auctions in the world...", totalAuctionCount))

	output.Conclude()
}
