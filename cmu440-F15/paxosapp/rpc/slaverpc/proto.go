// This file contains constants and arguments used to perform RPCs between
// Slave nodes and Master nodes.

package slaverpc

type GetArgs struct {
	Key string
}

type GetReply struct {
	Value []string
}

type AppendArgs struct {
	Key   string
	Value []string
}

type AppendReply struct {
	//Nothing to reply here
}
