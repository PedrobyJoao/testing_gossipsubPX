package libp2p

import (
	"github.com/PedrobyJoao/koko/internal/logger"
)

var zlog *logger.Logger

func init() {
	zlog = logger.NewZapLogger("pubsub")
}
