package bus

import (
	"context"
	"fmt"

	"cloud.google.com/go/pubsub"
	"github.com/sirupsen/logrus"
	"github.com/sotah-inc/server/app/pkg/logging"
)

func NewBus(projectID string, subscriberId string) (Bus, error) {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return Bus{}, err
	}

	return Bus{
		client:       client,
		context:      ctx,
		projectId:    projectID,
		subscriberId: subscriberId,
	}, nil
}

type Bus struct {
	context      context.Context
	projectId    string
	client       *pubsub.Client
	subscriberId string
}

func (b Bus) ResolveTopic(topicName string) (*pubsub.Topic, error) {
	topic := b.client.Topic(topicName)
	exists, err := topic.Exists(b.context)
	if err != nil {
		return nil, err
	}

	if exists {
		return topic, nil
	}

	return b.client.CreateTopic(b.context, topicName)
}

func (b Bus) resolveSubscription(topic *pubsub.Topic, subscriberName string) (*pubsub.Subscription, error) {
	subscription := b.client.Subscription(subscriberName)
	exists, err := subscription.Exists(b.context)
	if err != nil {
		return nil, err
	}

	if exists {
		return subscription, nil
	}

	return b.client.CreateSubscription(b.context, subscriberName, pubsub.SubscriptionConfig{Topic: topic})
}

func (b Bus) Subscribe(topicName string, stop chan interface{}, cb func(pubsub.Message)) error {
	topic, err := b.ResolveTopic(topicName)
	if err != nil {
		return err
	}

	subscriberName := fmt.Sprintf("subscriber-%s", b.subscriberId)

	entry := logging.WithFields(logrus.Fields{
		"subscriber-name": subscriberName,
		"topic":           topicName,
	})

	entry.Info("Subscribing to topic")
	sub, err := b.client.CreateSubscription(b.context, subscriberName, pubsub.SubscriptionConfig{Topic: topic})
	if err != nil {
		return err
	}

	cctx, cancel := context.WithCancel(b.context)
	go func() {
		<-stop

		entry.Info("Received stop signal, cancelling subscription")

		cancel()

		entry.Info("Stopping topic")
		topic.Stop()
	}()

	entry.Info("Waiting for messages")
	err = sub.Receive(cctx, func(ctx context.Context, msg *pubsub.Message) {
		msg.Ack()

		cb(*msg)
	})
	if err != nil {
		if err == context.Canceled {
			return nil
		}

		return err
	}

	return nil
}
