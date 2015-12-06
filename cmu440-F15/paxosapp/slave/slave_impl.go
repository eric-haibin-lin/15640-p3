package slave

import (
	"fmt"
	"github.com/cmu440-F15/paxosapp/rpc/slaverpc"
	"net"
	"net/http"
	"net/rpc"
	"sync"
)

type slaveNode struct {
	valuesMap     map[string][]string
	valuesMapLock *sync.Mutex
	srvId         int
	myHostPort    string
}

func NewSlaveNode(myHostPort string, srvId int) (SlaveNode, error) {
	var a slaverpc.RemoteSlaveNode
	node := slaveNode{}
	node.valuesMap = make(map[string][]string)
	node.valuesMapLock = &sync.Mutex{}
	node.srvId = srvId
	node.myHostPort = myHostPort

	listener, err := net.Listen("tcp", myHostPort)
	if err != nil {
		return nil, err
	}

	a = &node

	err = rpc.RegisterName("SlaveNode", slaverpc.Wrap(a))
	if err != nil {
		return nil, err
	}

	rpc.HandleHTTP()
	go http.Serve(listener, nil)

	return a, nil
}

func (sn *slaveNode) Append(args *slaverpc.AppendArgs, reply *slaverpc.AppendReply) error {
	fmt.Println("Append invoked on ", sn.srvId)
	defer fmt.Println("Leaving Append on ", sn.srvId)
	sn.valuesMapLock.Lock()
	defer sn.valuesMapLock.Unlock()
	key := args.Key
	value := args.Value
	sn.valuesMap[key] = append(sn.valuesMap[key], value)
	return nil
}

func (sn *slaveNode) Get(args *slaverpc.GetArgs, reply *slaverpc.GetReply) error {
	fmt.Println("Get invoked on ", sn.srvId)
	defer fmt.Println("Leaving Get on ", sn.srvId)
	sn.valuesMapLock.Lock()
	defer sn.valuesMapLock.Unlock()
	key := args.Key
	value := sn.valuesMap[key]
	reply.Value = value
	return nil
}
