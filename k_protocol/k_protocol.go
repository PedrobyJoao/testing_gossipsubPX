package k_protocol

import (
	"context"
	"encoding/json"
)

type NodeInfo struct {
	PeerID     string
	Multiaddrs []string
	Resources  Resources
}

type Resources struct {
	CPU int
	RAM int
}

var IndexOfCPs = make(map[string]NodeInfo)

type CPTopicHandler struct{}

func (h *CPTopicHandler) Handle(ctx context.Context, msg []byte) {
	var nodeInfo NodeInfo
	err := json.Unmarshal(msg, &nodeInfo)
	if err != nil {
		zlog.Sugar().Errorf("Failed to unmarshal to nodeInfo: %v", err)
		return
	}

	zlog.Sugar().Infof("Adding nodeInfo to the index: %+v", nodeInfo)
	IndexOfCPs[nodeInfo.PeerID] = nodeInfo
}
