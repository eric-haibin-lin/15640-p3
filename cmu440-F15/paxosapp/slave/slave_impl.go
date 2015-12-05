package slave

import (
	"errors"
	"github.com/cmu440-F15/paxosapp/rpc/slaverpc"
)

type slaveNode struct {
}

func NewSlaveNode(myHostPort string, srvId int) (SlaveNode, error) {
	return nil, errors.New("Not implemented yet.")
}

func (cn *slaveNode) Append(args *slaverpc.AppendArgs, reply *slaverpc.AppendReply) error{
	return errors.New("Not implemented yet.")
}

func (cn *slaveNode) Get(args *slaverpc.GetArgs, reply *slaverpc.GetReply) error{
	return errors.New("Not implemented yet.")
}