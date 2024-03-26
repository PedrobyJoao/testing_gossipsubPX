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
	// "github.com/libp2p/go-libp2p/p2p/host/autorelay"
	// "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/multiformats/go-multiaddr"
	mafilt "github.com/whyrusleeping/multiaddr-filter"
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

	filter := multiaddr.NewFilters()
	for _, s := range defaultServerFilters {
		f, err := mafilt.NewMask(s)
		if err != nil {
			zlog.Sugar().Errorf("incorrectly formatted address filter in config: %s - %v", s, err)
		}
		filter.AddFilter(*f, multiaddr.ActionDeny)
	}

	h, err = libp2p.New(
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic", port),
		),
		libp2p.Identity(prvKey),
		// libp2p.ProtocolVersion(identify.IDPush),
		libp2p.AddrsFactory(makeAddrsFactory([]string{}, []string{}, defaultServerFilters)),
		libp2p.ConnectionGater((*filtersConnectionGater)(filter)),
		// libp2p.EnableNATService(),
		// libp2p.DefaultTransports,
		// libp2p.EnableNATService(),
		// libp2p.EnableRelay(),
		// libp2p.EnableHolePunching(),
		// libp2p.EnableRelayService(
		// 	relay.WithResources(
		// 		relay.Resources{
		// 			MaxReservations:        256,
		// 			MaxCircuits:            32,
		// 			BufferSize:             4096,
		// 			MaxReservationsPerPeer: 8,
		// 			MaxReservationsPerIP:   16,
		// 		},
		// 	),
		// 	relay.WithLimit(&relay.RelayLimit{
		// 		Duration: 5 * time.Minute,
		// 		Data:     1 << 21, // 2 MiB
		// 	}),
		// ),
		// libp2p.EnableAutoRelayWithPeerSource(
		// 	func(ctx context.Context, num int) <-chan peer.AddrInfo {
		// 		r := make(chan peer.AddrInfo)
		// 		go func() {
		// 			defer close(r)
		// 			for i := 0; i < num; i++ {
		// 				select {
		// 				case p := <-newPeer:
		// 					select {
		// 					case r <- p:
		// 					case <-ctx.Done():
		// 						return
		// 					}
		// 				case <-ctx.Done():
		// 					return
		// 				}
		// 			}
		// 		}()
		// 		return r
		// 	},
		// 	autorelay.WithBootDelay(time.Minute),
		// 	autorelay.WithBackoff(30*time.Second),
		// 	autorelay.WithMinCandidates(2),
		// 	autorelay.WithMaxCandidates(3),
		// 	autorelay.WithNumRelays(2),
		// ),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create new p2p host: %v", err)
	}

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
		// params.Dscore = 0
	}

	opts := []libp2pPS.Option{
		libp2pPS.WithFloodPublish(true),
		libp2pPS.WithPeerExchange(true),
		libp2pPS.WithGossipSubParams(params),
	}

	return opts
}

func makeAddrsFactory(announce []string, appendAnnouce []string, noAnnounce []string) func([]multiaddr.Multiaddr) []multiaddr.Multiaddr {
	var err error                     // To assign to the slice in the for loop
	existing := make(map[string]bool) // To avoid duplicates

	annAddrs := make([]multiaddr.Multiaddr, len(announce))
	for i, addr := range announce {
		annAddrs[i], err = multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil
		}
		existing[addr] = true
	}

	var appendAnnAddrs []multiaddr.Multiaddr
	for _, addr := range appendAnnouce {
		if existing[addr] {
			// skip AppendAnnounce that is on the Announce list already
			continue
		}
		appendAddr, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil
		}
		appendAnnAddrs = append(appendAnnAddrs, appendAddr)
	}

	filters := multiaddr.NewFilters()
	noAnnAddrs := map[string]bool{}
	for _, addr := range noAnnounce {
		f, err := mafilt.NewMask(addr)
		if err == nil {
			filters.AddFilter(*f, multiaddr.ActionDeny)
			continue
		}
		maddr, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil
		}
		noAnnAddrs[string(maddr.Bytes())] = true
	}

	return func(allAddrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
		var addrs []multiaddr.Multiaddr
		if len(annAddrs) > 0 {
			addrs = annAddrs
		} else {
			addrs = allAddrs
		}
		addrs = append(addrs, appendAnnAddrs...)

		var out []multiaddr.Multiaddr
		for _, maddr := range addrs {
			// check for exact matches
			ok := noAnnAddrs[string(maddr.Bytes())]
			// check for /ipcidr matches
			if !ok && !filters.AddrBlocked(maddr) {
				out = append(out, maddr)
			}
		}
		return out
	}
}
