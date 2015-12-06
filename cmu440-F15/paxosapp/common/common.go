package common

import (
	"net/rpc"
)

// A Conn struct contains the hostPort of connection endpoint and the dialer 
// to this hostport 
type Conn struct {
	HostPort []string
	Dialer *rpc.Client 
}