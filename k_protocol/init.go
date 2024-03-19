package k_protocol

import (
	"github.com/PedrobyJoao/koko/internal/logger"
)

var zlog *logger.Logger

func init() {
	zlog = logger.NewZapLogger("k_protocol")
}
