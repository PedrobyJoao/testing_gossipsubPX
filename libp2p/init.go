package libp2p

import (
	"github.com/PedrobyJoao/koko/internal/logger"

	"github.com/ipfs/go-log/v2"
)

var zlog *logger.Logger

func init() {
	zlog = logger.NewZapLogger("pubsub")

	// log.SetLogLevel("go-libp2p", "debug")
	// log.SetLogLevel("net/host", "debug")
	// log.SetLogLevel("net/peerstore", "debug")
	// log.SetLogLevel("net/record", "debug")
	log.SetLogLevel("pubsub", "debug")
	// log.SetLogLevel("net/identify", "debug")
}
