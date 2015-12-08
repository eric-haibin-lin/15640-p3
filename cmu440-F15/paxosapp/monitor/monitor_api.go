package monitor

import (
	"github.com/cmu440-F15/paxosapp/rpc/monitorrpc"
)

type MonitorNode interface {
	HeartBeat(args *monitorrpc.HeartBeatArgs, reply *monitorrpc.HeartBeatReply) error
}
