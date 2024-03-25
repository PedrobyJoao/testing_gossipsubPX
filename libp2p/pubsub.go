package libp2p

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	libp2pPS "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

type MsgHandler interface {
	Handle(ctx context.Context, msg []byte)
}

// PubSub is basically a wrapper around *libp2p.PubSub original struct
// which has one additional method for joining and subscribing to a given topic
type PubSub struct {
	*libp2pPS.PubSub
	hostID            string
	joinSubscribeOnce sync.Once

	// Keeps track of subscribed topics returning the PsTopicSubscription object
	// with the related topic methods. It's useful when we try to publish in a given
	// topic from different places that do not have access to the *libp2p.topic/sub
	// object
	SubscribedTopics map[string]*PsTopicSubscription
}

// PsTopicSubscription is returned when the host joins/subs to a topic using the
// created JoinSubscribeTopic method. With its values, the host can deal with all
// the available attributes and methods for the given *libp2p.Topic and *libp2p.Subscribe.
type PsTopicSubscription struct {
	Topic  *libp2pPS.Topic
	events *libp2pPS.TopicEventHandler
	Sub    *libp2pPS.Subscription

	msgHandler MsgHandler

	hostID string

	closeOnce sync.Once
}

var (
	pubsubHost   *PubSub
	onceGossipPS sync.Once
)

// NewGossipPubSub creates a new GossipSub instance with the given host or returns
// an existing one if it has been previously created.
// Note: it must be called before connected to peers, otherwise, peers might be invisible
// within the entered topics
func NewGossipPubSub(ctx context.Context, host host.Host, opts ...libp2pPS.Option) (*PubSub, error) {
	var err error

	if pubsubHost != nil {
		return pubsubHost, nil
	}

	onceGossipPS.Do(func() {
		gs, err := libp2pPS.NewGossipSub(ctx, host, opts...)
		if err != nil {
			err = fmt.Errorf("failed to create gossipsub: %v", err)
			return
		}
		pubsubHost = &PubSub{
			gs,
			host.ID().String(),
			sync.Once{},
			map[string]*PsTopicSubscription{},
		}

	})

	return pubsubHost, err
}

// JoinSubscribeTopic joins the given topic and subscribes to the topic.
func (ps *PubSub) JoinSubscribeTopic(ctx context.Context, topicName string,
	msgHandler MsgHandler) (*PsTopicSubscription, error) {
	// TODO: I'm not sure if having both methods called in the same function is a good thing.
	// We'll discover along the way (I did this because they seem to be highly coupled)
	var err error

	if st, ok := ps.SubscribedTopics[topicName]; ok {
		// Topic already subscribed and object *libp2p.topic/sub created, return it
		return st, nil
	}

	var topicSub *PsTopicSubscription
	ps.joinSubscribeOnce.Do(func() {
		tp, err := ps.Join(topicName)
		if err != nil {
			err = fmt.Errorf("Failed to join topic %v, Error: %v", topicName, err)
			return
		}

		var events *libp2pPS.TopicEventHandler
		events, err = tp.EventHandler()
		if err != nil {
			err = fmt.Errorf("Couldn't create topic's event handler for %v, Error: %v",
				topicName, err)
			return
		}
		go logEvents(ctx, events)

		sub, err := tp.Subscribe()
		if err != nil {
			err = fmt.Errorf("Failed to subscribe to topic %v, Error: %v", topicName, err)
			return
		}
		zlog.Sugar().Debugf("Joined and subscribe to topic: %v", topicName)

		topicSub = &PsTopicSubscription{
			Topic:      tp,
			Sub:        sub,
			events:     events,
			hostID:     ps.hostID,
			msgHandler: msgHandler,
		}

		ps.SubscribedTopics[topicName] = topicSub
	})

	if err != nil {
		return nil, err
	}

	if topicSub == nil {
		return nil, fmt.Errorf("Topic already joined and/or subscribed!")
	} else {
		go topicSub.ListenForMessages(ctx)
	}

	return topicSub, nil
}

// Publish publishes the given message to the objected topic
func (ts *PsTopicSubscription) Publish(msg any) error {
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("Failed to marshal message, Error: %v", err)
	}

	err = ts.Topic.Publish(context.Background(), msgBytes)
	if err != nil {
		return fmt.Errorf("Failed to publish message, Error: %v", err)
	}
	zlog.Sugar().Debugf("Published message: %v", string(msgBytes))

	return nil
}

// ListenForMessages receives a channel of type *libp2pPS.Message and it sends
// all the messages received for a given topic to this channel. Ignoring
// the messages send by the host
func (ts *PsTopicSubscription) ListenForMessages(ctx context.Context) {
	for {
		zlog.Sugar().Debugf("Waiting for message for topic: %v", ts.Topic.String())
		msg, err := ts.Sub.Next(ctx)
		if err != nil {
			zlog.Sugar().Infof("Libp2p Pubsub topic %v done: %v", ts.Topic.String(), err)
			return
		}

		if msg.GetFrom().String() == ts.hostID {
			continue
		}

		zlog.Sugar().Debugf("(%v): %v", msg.GetFrom().String(), msg.Message.Data)
		if ts.msgHandler != nil {
			go ts.msgHandler.Handle(ctx, msg.GetData())
		} else {
			zlog.Sugar().Debugf("No message handler for topic: %v", ts.Topic.String())
		}
	}
}

func logEvents(ctx context.Context, eventHandler *libp2pPS.TopicEventHandler) {
	for {
		ev, err := eventHandler.NextPeerEvent(ctx)
		if err != nil {
			zlog.Sugar().Debug("Error trying to get next event from topic")
			eventHandler.Cancel()
			return
		}

		var typeString string
		if ev.Type == libp2pPS.PeerJoin {
			typeString = "JOIN"
		} else {
			typeString = "LEAVE"
		}

		zlog.Sugar().Debugf("Peer: %v, Event: %v", ev.Peer.String(), typeString)
	}
}

// Unsubscribe unsubscribes from the topic subscription.
func (ts *PsTopicSubscription) Unsubscribe() {
	ts.Sub.Cancel()
}

// Close closes both subscription and topic for the given values assigned
// to ts *PsTopicSubscription
func (ts *PsTopicSubscription) Close(ctx context.Context) error {
	var err error
	ts.closeOnce.Do(func() {
		if ts.events != nil {
			ts.events.Cancel()
		}
		if ts.Sub != nil {
			ts.Sub.Cancel()
		}
		if ts.Topic != nil {
			err = ts.Topic.Close()
		}
		zlog.Sugar().Infof("Closed subscription and topic itself for topic: %v", ts.Topic.String())
	})

	if err != nil {
		return fmt.Errorf("For peer %v, Error: %w", ts.hostID, err)
	}

	return nil
}
