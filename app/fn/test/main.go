package test

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/sotah-inc/server/app/pkg/bus"
	"github.com/sotah-inc/server/app/pkg/bus/codes"
	"github.com/sotah-inc/server/app/pkg/state/subjects"
)

var projectId = os.Getenv("GCP_PROJECT")
var bu bus.Bus

func init() {
	var err error
	bu, err = bus.NewBus(projectId, "fn-test")
	if err != nil {
		log.Fatalf("Failed to create new bus: %s", err.Error())

		return
	}
}

func HelloHTTP(w http.ResponseWriter, r *http.Request) {
	msg, err := bu.RequestFromTopic(string(subjects.Boot), "world", 5*time.Second)
	if err != nil {
		http.Error(w, "Error sending boot request", http.StatusInternalServerError)

		return
	}

	if msg.Code != codes.Ok {
		http.Error(w, fmt.Sprintf("Response was not Ok: %s", msg.Err), http.StatusInternalServerError)

		return
	}

	fmt.Fprintf(w, "Published msg: %v", msg.Data)
}
