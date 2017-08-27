package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/ihsw/sotah-server/app/subjects"

	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	// parsing the command flags
	var (
		app            = kingpin.New("sotah-server", "A command-line Blizzard AH client.")
		natsHost       = app.Flag("nats-host", "NATS hostname").Default("localhost").OverrideDefaultFromEnvar("NATS_HOST").Short('h').String()
		natsPort       = app.Flag("nats-port", "NATS port").Default("4222").OverrideDefaultFromEnvar("NATS_PORT").Short('p').Int()
		configFilepath = app.Flag("config", "Relative path to config json").Required().Short('c').String()
		apiKey         = app.Flag("api-key", "Blizzard Mashery API key").OverrideDefaultFromEnvar("API_KEY").String()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	// loading the config file
	c, err := newConfigFromFilepath(*configFilepath)
	if err != nil {
		log.Fatalf("Could not fetch config: %s\n", err.Error())

		return
	}

	// loading a resolver with the config
	// res := newResolver(c)

	// optionally overriding api key in config
	if len(*apiKey) > 0 {
		c.APIKey = *apiKey
	}

	// connecting the messenger
	mess, err := newMessenger(*natsHost, *natsPort)
	if err != nil {
		log.Fatalf("Could not connect messenger: %s\n", err.Error())

		return
	}

	// establishing a state and filling it with statuses
	sta := NewState(c, mess)
	for _, reg := range c.Regions {
		stat, err := NewStatusFromFilepath(reg, "./src/github.com/ihsw/sotah-server/app/TestData/realm-status.json")
		if err != nil {
			log.Fatalf("Could not fetch statuses from http: %s\n", err.Error())

			return
		}

		sta.Statuses[reg.Name] = stat
	}

	// listening for status requests
	stopChans := map[string]chan interface{}{
		subjects.Status:            make(chan interface{}),
		subjects.Regions:           make(chan interface{}),
		subjects.GenericTestErrors: make(chan interface{}),
	}
	if err := sta.ListenForStatus(stopChans[subjects.Status]); err != nil {
		log.Fatalf("Could not listen for status requests: %s\n", err.Error())

		return
	}
	if err := sta.listenForRegions(stopChans[subjects.Regions]); err != nil {
		log.Fatalf("Could not listen for regions requests: %s\n", err.Error())

		return
	}
	if err := sta.listenForGenericTestErrors(stopChans[subjects.GenericTestErrors]); err != nil {
		log.Fatalf("Could not listen for generic test errors requests: %s\n", err.Error())

		return
	}

	fmt.Printf("Running!\n")

	// catching SIGINT
	sigIn := make(chan os.Signal, 1)
	signal.Notify(sigIn, os.Interrupt)
	<-sigIn
	fmt.Printf("Caught SIGINT!\n")

	// stopping listeners
	for _, stop := range stopChans {
		stop <- struct{}{}
	}

	// exiting
	os.Exit(0)
}
