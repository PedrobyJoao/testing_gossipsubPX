package libp2p

import (
	"log"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func GetPeersFromPeerStore() peer.IDSlice {
	log.Printf(
		"printing connected peers just in case it's different from peerstore\n Connected: %v\n",
		h.Network().Peers(),
	)
	return h.Peerstore().Peers()
}

func DebugConnectedPeers() {
	// with ticker
	t := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-t.C:
			log.Printf(
				"\nPeerStore: %v\n",
				h.Peerstore().Peers(),
			)

			// cab, ok := peerstore.GetCertifiedAddrBook(h.Peerstore())
			// if !ok {
			// 	zlog.Sugar().Error("peerstore does not implement CertifiedAddrBook")
			// }
			// for _, p := range h.Peerstore().Peers() {
			// 	rec := cab.GetPeerRecord(p)
			// 	if rec == nil {
			// 		zlog.Sugar().Errorf("Peer %s has NO signed peer record", p)
			// 	} else {
			// 		zlog.Sugar().Debugf("Peer %s HAS signed peer record", p)
			// 	}
			// }

			log.Printf(
				"\nConnected peers: %v\n",
				h.Network().Peers(),
			)
		}
	}
}
