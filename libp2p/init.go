package libp2p

import (
	"github.com/PedrobyJoao/koko/internal/logger"

	"github.com/ipfs/go-log/v2"
)

var zlog *logger.Logger

func init() {
	zlog = logger.NewZapLogger("pubsub")

	log.SetLogLevel("pubsub", "debug")
}
