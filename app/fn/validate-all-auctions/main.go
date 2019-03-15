package validate_all_auctions

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/sotah-inc/server/app/pkg/bus"
	"github.com/sotah-inc/server/app/pkg/bus/codes"
	"github.com/sotah-inc/server/app/pkg/logging"
	"github.com/sotah-inc/server/app/pkg/state/subjects"
)

var projectId = os.Getenv("GCP_PROJECT")

var busClient bus.Client
var validateAuctionsTopic *pubsub.Topic

func init() {
	var err error
	busClient, err = bus.NewClient(projectId, "fn-compute-all-live-auctions")
	if err != nil {
		log.Fatalf("Failed to create new bus client: %s", err.Error())

		return
	}
	validateAuctionsTopic, err = busClient.FirmTopic(string(subjects.ValidateAuctions))
	if err != nil {
		log.Fatalf("Failed to get firm topic: %s", err.Error())

		return
	}
}

type PubSubMessage struct {
	Data []byte `json:"data"`
}

func ValidateAllLiveAuctions(_ context.Context, m PubSubMessage) error {
	var in bus.Message
	if err := json.Unmarshal(m.Data, &in); err != nil {
		return err
	}

	// producing region-realm-timestamp tuples from bus message
	tuples, err := bus.NewRegionRealmTimestampTuples(in.Data)
	if err != nil {
		return err
	}

	// converting to messages for requesting
	messages, err := tuples.ToMessages()
	if err != nil {
		return err
	}

	// enqueueing them and gathering result jobs
	responseItems, err := busClient.BulkRequest(validateAuctionsTopic, messages, 200*time.Second)
	if err != nil {
		return err
	}

	for _, msg := range responseItems {
		if msg.Code != codes.Ok {
			logging.WithField("msg", msg).Error("Received erroneous response")

			continue
		}
	}

	return nil
}
