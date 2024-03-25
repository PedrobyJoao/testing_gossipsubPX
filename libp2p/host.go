package libp2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"time"

	"github.com/PedrobyJoao/koko/db"
	"github.com/PedrobyJoao/koko/k_protocol"
	"github.com/PedrobyJoao/koko/models"

	"github.com/libp2p/go-libp2p"
	libp2pPS "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/record"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/multiformats/go-multiaddr"
)

var (
	h       host.Host
	ps      *PubSub
	newPeer = make(chan peer.AddrInfo)
)

// NewHost creates a new libp2p host
func NewHost(port int, bootstrapPeers []string, cp bool) (host.Host, error) {
	ctx := context.Background()

	// Creates a new RSA key pair for this host.
	r := rand.Reader
	prvKey, pubKey, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	hostInfo, err := retrieveHostInfoDB()
	if err != nil {
		err = saveHostInfoDB(prvKey, pubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to save host info to DB: %v", err)
		}
	} else {
		prvKey, err = crypto.UnmarshalPrivateKey(hostInfo.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal private key: %v", err)
		}
	}

	h, err = libp2p.New(
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic", port),
		),
		libp2p.Identity(prvKey),
		libp2p.EnableNATService(),
		libp2p.DefaultTransports,
		libp2p.EnableNATService(),
		libp2p.EnableRelay(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelayService(
			relay.WithResources(
				relay.Resources{
					MaxReservations:        256,
					MaxCircuits:            32,
					BufferSize:             4096,
					MaxReservationsPerPeer: 8,
					MaxReservationsPerIP:   16,
				},
			),
			relay.WithLimit(&relay.RelayLimit{
				Duration: 5 * time.Minute,
				Data:     1 << 21, // 2 MiB
			}),
		),
		libp2p.EnableAutoRelayWithPeerSource(
			func(ctx context.Context, num int) <-chan peer.AddrInfo {
				r := make(chan peer.AddrInfo)
				go func() {
					defer close(r)
					for i := 0; i < num; i++ {
						select {
						case p := <-newPeer:
							select {
							case r <- p:
							case <-ctx.Done():
								return
							}
						case <-ctx.Done():
							return
						}
					}
				}()
				return r
			},
			autorelay.WithBootDelay(time.Minute),
			autorelay.WithBackoff(30*time.Second),
			autorelay.WithMinCandidates(2),
			autorelay.WithMaxCandidates(3),
			autorelay.WithNumRelays(2),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create new p2p host: %v", err)
	}

	err = addSignedRecord(h)
	if err != nil {
		return nil, fmt.Errorf("failed to add signed record: %v", err)
	}

	// Monkey patch the identify protocol to allow discovering advertised addresses of networks of 3 or more nodes, instead of 5.
	// Setting the value to 2 means two other nodes must see the same addr for a node to discover its observed addr, which enables a network
	// of at least 3 nodes.
	identify.ActivationThresh = 2

	gsOpts := configureGossipSubOpts(len(bootstrapPeers) == 0)

	ps, err = NewGossipPubSub(
		ctx,
		h,
		gsOpts...,
	)
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

		zlog.Sugar().Debugf("Connecting to bootstrap peer: %v", addr)
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

func retrieveHostInfoDB() (*models.HostInfo, error) {
	var hostInfo models.HostInfo
	r := db.DB.First(&hostInfo)
	if r.Error != nil {
		return nil, fmt.Errorf("failed to retrieve host info from DB: %v", r.Error)
	}

	return &hostInfo, nil
}

func saveHostInfoDB(privKey crypto.PrivKey, pubKey crypto.PubKey) error {
	privKeyBytes, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %v", err)
	}

	pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %v", err)
	}

	hostInfo := &models.HostInfo{
		ID:         1,
		PrivateKey: privKeyBytes,
		PublicKey:  pubKeyBytes,
	}

	r := db.DB.Save(&hostInfo)
	if r.Error != nil {
		return fmt.Errorf("failed to save host info to DB: %v", r.Error)
	}

	return nil
}

func configureGossipSubOpts(isBootstrap bool) []libp2pPS.Option {
	params := libp2pPS.DefaultGossipSubParams()

	if isBootstrap {
		params.D = 0
		params.Dhi = 0
		params.Dlo = 0
		params.Dout = 0
	}

	opts := []libp2pPS.Option{
		libp2pPS.WithFloodPublish(true),
		libp2pPS.WithPeerExchange(true),
		libp2pPS.WithGossipSubParams(params),
	}

	return opts
}

func addSignedRecord(h host.Host) error {
	ttl := time.Hour * 24

	// Create a new PeerRecord
	rec := peer.NewPeerRecord()
	rec.PeerID = h.ID()
	rec.Addrs = h.Addrs()

	// Sign the PeerRecord using the peer's private key
	envelope, err := record.Seal(rec, h.Peerstore().PrivKey(h.ID()))
	if err != nil {
		return fmt.Errorf("failed to sign peer record: %v", err)
	}

	// Marshal the signed Envelope
	signedRecordBytes, err := envelope.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal signed record: %v", err)
	}

	// Consume the signed Envelope and extract the PeerRecord
	envelope, _, err = record.ConsumeEnvelope(signedRecordBytes, peer.PeerRecordEnvelopeDomain)
	if err != nil {
		return fmt.Errorf("failed to consume signed record: %v", err)
	}

	// peerRec := untypedRecord.(*peer.PeerRecord)

	certifiedAddrBook, ok := peerstore.GetCertifiedAddrBook(h.Peerstore())
	if !ok {
		return fmt.Errorf("failed to get certified address book")
	}

	// Add the certified addresses from the PeerRecord to the CertifiedAddrBook
	accepted, err := certifiedAddrBook.ConsumePeerRecord(envelope, ttl)
	if err != nil {
		return fmt.Errorf("failed to consume peer record: %v", err)
	}

	if accepted {
		zlog.Sugar().Debugf("Peer record accepted")
	} else {
		zlog.Sugar().Error("Peer record rejected")
	}

	zlog.Sugar().Debugf("Peer record: %v", certifiedAddrBook.GetPeerRecord(h.ID()))
	return nil
}
