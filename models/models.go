package models

type HostInfo struct {
	ID         int
	PrivateKey []byte
	PublicKey  []byte
}
