package libp2p

import (
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

var defaultServerFilters = []string{
	"/ip4/10.0.0.0/ipcidr/8",
	"/ip4/100.64.0.0/ipcidr/10",
	"/ip4/169.254.0.0/ipcidr/16",
	"/ip4/172.16.0.0/ipcidr/12",
	"/ip4/192.0.0.0/ipcidr/24",
	"/ip4/192.0.2.0/ipcidr/24",
	"/ip4/192.168.0.0/ipcidr/16",
	"/ip4/198.18.0.0/ipcidr/15",
	"/ip4/198.51.100.0/ipcidr/24",
	"/ip4/203.0.113.0/ipcidr/24",
	"/ip4/240.0.0.0/ipcidr/4",
	"/ip6/100::/ipcidr/64",
	"/ip6/2001:2::/ipcidr/48",
	"/ip6/2001:db8::/ipcidr/32",
	"/ip6/fc00::/ipcidr/7",
	"/ip6/fe80::/ipcidr/10",
}

type filtersConnectionGater ma.Filters

var _ connmgr.ConnectionGater = (*filtersConnectionGater)(nil)

func (f *filtersConnectionGater) InterceptAddrDial(_ peer.ID, addr ma.Multiaddr) (allow bool) {
	return !(*ma.Filters)(f).AddrBlocked(addr)
}

func (f *filtersConnectionGater) InterceptPeerDial(p peer.ID) (allow bool) {
	return true
}

func (f *filtersConnectionGater) InterceptAccept(connAddr network.ConnMultiaddrs) (allow bool) {
	return !(*ma.Filters)(f).AddrBlocked(connAddr.RemoteMultiaddr())
}

func (f *filtersConnectionGater) InterceptSecured(_ network.Direction, _ peer.ID, connAddr network.ConnMultiaddrs) (allow bool) {
	return !(*ma.Filters)(f).AddrBlocked(connAddr.RemoteMultiaddr())
}

func (f *filtersConnectionGater) InterceptUpgraded(_ network.Conn) (allow bool, reason control.DisconnectReason) {
	return true, 0
}
