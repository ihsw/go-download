package command

import (
	"os"
	"os/signal"

	"github.com/sotah-inc/server/app/pkg/logging"
	"github.com/sotah-inc/server/app/pkg/sotah"
	"github.com/sotah-inc/server/app/pkg/state"
)

func Api(config state.APIStateConfig) error {
	logging.Info("Starting api")

	// establishing a state
	apiState, err := state.NewAPIState(config)
	if err != nil {
		return err
	}

	// opening all listeners
	if err := apiState.Listeners.Listen(); err != nil {
		return err
	}

	// starting up a collector
	collectorStop := make(sotah.WorkerStopChan)
	onCollectorStop := apiState.StartCollector(collectorStop)

	// catching SIGINT
	logging.Info("Waiting for SIGINT")
	sigIn := make(chan os.Signal, 1)
	signal.Notify(sigIn, os.Interrupt)
	<-sigIn

	logging.Info("Caught SIGINT, exiting")

	// stopping listeners
	apiState.Listeners.Stop()

	logging.Info("Stopping collector")
	collectorStop <- struct{}{}

	logging.Info("Waiting for collector to stop")
	<-onCollectorStop

	logging.Info("Exiting")
	return nil
}