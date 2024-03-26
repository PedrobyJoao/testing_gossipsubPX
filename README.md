If you're relying **ONLY** on Gossipsub `Peer Exchange (PX)` to handle the discovery of your libp2p network, it won't
work out of the box (at least using the golang implementation). 
See the following issue to understand why: https://github.com/libp2p/go-libp2p/issues/2754

To make it work, I used my own fork with some additional modifications to the handlers of `Identify` family of protocol. 
Fork: https://github.com/PedrobyJoao/go-libp2p/tree/identify_add_to_certifiedBookAddr
