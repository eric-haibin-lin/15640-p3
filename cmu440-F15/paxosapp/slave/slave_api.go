package slave

import (
	"github.com/cmu440-F15/paxosapp/rpc/slaverpc"
)

type SlaveNode interface {
	Append(args *slaverpc.AppendArgs, reply *slaverpc.AppendReply) error
	Get(args *slaverpc.GetArgs, reply *slaverpc.GetReply) error
}
