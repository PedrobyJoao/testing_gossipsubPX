package libp2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"time"

	"github.com/PedrobyJoao/koko/k_protocol"
	"github.com/libp2p/go-libp2p"
	libp2pPS "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

var (
	h  host.Host
	ps *PubSub
)

// NewHost creates a new libp2p host
func NewHost(port int, bootstrapPeers []string, cp bool) (host.Host, error) {
	ctx := context.Background()

	// Creates a new RSA key pair for this host.
	r := rand.Reader
	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	h, err = libp2p.New(
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port),
			fmt.Sprintf("/ip4/127.0.0.1/udp/%d/quic", port),
		),
		libp2p.Identity(prvKey),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create new p2p host: %v", err)
	}

	var gossipsubOpts []libp2pPS.Option
	if len(bootstrapPeers) == 0 {
		gossipsubOpts = configurePSAsBootstrapNode(ctx, h)
	}

	ps, err = NewGossipPubSub(ctx, h, gossipsubOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gossipsub: %v", err)
	}

	ps.JoinSubscribeTopic(ctx, "compute-providers", &k_protocol.CPTopicHandler{})

	if cp {
		go periodicallyPublishNodeInfo(h, "compute-providers")

	}

	if len(bootstrapPeers) != 0 {
		err = connectToPeers(ctx, h, bootstrapPeers)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to bootstrap nodes: %v", err)
		}
	}

	// prints host PeerID and listen addresses
	zlog.Sugar().Debugf("PeerID: %s\n", h.ID())
	zlog.Sugar().Debugf("Listen addresses: %s\n", h.Addrs())
	go DebugConnectedPeers()

	return h, nil
}

func periodicallyPublishNodeInfo(h host.Host, topic string) {
	zlog.Sugar().Debugf("Starting background task to publish nodeinfo to topic %s", topic)
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			zlog.Sugar().Debugf("Publishing nodeInfo to topic %s", topic)
			var multaddrsStr []string
			for _, addr := range h.Addrs() {
				multaddrsStr = append(multaddrsStr, addr.String())
			}
			nodeInfo := &k_protocol.NodeInfo{
				PeerID:     h.ID().String(),
				Multiaddrs: multaddrsStr,
				Resources: k_protocol.Resources{
					CPU: 100,
					RAM: 8000,
				},
			}

			if subscription, ok := pubsubHost.SubscribedTopics[topic]; ok {
				err := subscription.Publish(nodeInfo)
				if err != nil {
					zlog.Sugar().Errorf("Failed to publish nodeInfo: ", err)
				}
				zlog.Sugar().Debugf("nodeInfo successfully published to %s", topic)
			} else {
				zlog.Sugar().Errorf("Topic %s not subscribed", topic)
			}
		}
	}
}

// connectToPeers connects to a list of peers, example of peer string format:
// "/ip4/127.0.0.1/tcp/8080/p2p/QmUJ52zADKoGSTesbvAfqX7BfidF8NxDQ2h6pndNQuAHmV"
func connectToPeers(ctx context.Context, h host.Host, peers []string) error {
	for _, bp := range peers {
		addr, err := multiaddr.NewMultiaddr(bp)
		if err != nil {
			return fmt.Errorf("failed to parse bootstrap peer address to multiaddr: %v", err)
		}

		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			return fmt.Errorf("failed to parse peer address (%v) to peerInfo: %v", addr, err)
		}

		err = h.Connect(ctx, *peerInfo)
		if err != nil {
			return fmt.Errorf("failed to connect to bootstrap peer: %v", err)
		} else {
			log.Println("connected to bootstrap peer: ", peerInfo.ID)
		}
	}
	return nil
}
