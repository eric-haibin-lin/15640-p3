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
	valuesMap     map[string][][]string
	ranksMap      map[string]float64
	valuesMapLock *sync.Mutex
	ranksMapLock  *sync.Mutex
	srvId         int
	myHostPort    string
}

func NewSlaveNode(myHostPort string, srvId int) (SlaveNode, error) {
	fmt.Println("NewSlaveNode invoked on", srvId)
	var a slaverpc.RemoteSlaveNode
	node := slaveNode{}
	node.valuesMap = make(map[string][][]string)
	node.ranksMap = make(map[string]float64)
	node.valuesMapLock = &sync.Mutex{}
	node.ranksMapLock = &sync.Mutex{}
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
	fmt.Println("Append invoked on ", sn.srvId, " for Key: ", args.Key)
	defer fmt.Println("Leaving Append on ", sn.srvId)
	sn.valuesMapLock.Lock()
	defer sn.valuesMapLock.Unlock()
	key := args.Key
	value := args.Value
	_, ok := sn.valuesMap[key]

	if !ok {
		fmt.Println("Didn't find anything on slave ", sn.srvId, ", so creating a new slice")
		sn.valuesMap[key] = make([][]string, 0)
	}

	//fmt.Println("Slave ", sn.srvId, " appended for key: ", args.Key)
	sn.valuesMap[key] = append(sn.valuesMap[key], value)
	return nil
}

func (sn *slaveNode) Get(args *slaverpc.GetArgs, reply *slaverpc.GetReply) error {
	fmt.Println("Get invoked on ", sn.srvId, " for key : ", args.Key)
	defer fmt.Println("Leaving Get on ", sn.srvId)
	sn.valuesMapLock.Lock()
	defer sn.valuesMapLock.Unlock()
	key := args.Key
	value, ok := sn.valuesMap[key]
	if ok {
		reply.Status = 1
		reply.Value = value[len(value)-1]
	} else {
		reply.Status = 0
	}
	return nil
}

func (sn *slaveNode) GetRank(args *slaverpc.GetRankArgs, reply *slaverpc.GetRankReply) error {
	fmt.Println("GetRank invoked on ", sn.srvId)
	defer fmt.Println("Leaving GetRank on ", sn.srvId)
	sn.ranksMapLock.Lock()
	defer sn.ranksMapLock.Unlock()
	key := args.Key
	value := sn.ranksMap[key]
	fmt.Println("Rank found for key", key, ": ", value)
	reply.Value = value
	return nil
}

func (sn *slaveNode) PutRank(args *slaverpc.PutRankArgs, reply *slaverpc.PutRankReply) error {
	fmt.Println("PutRank invoked on ", sn.srvId)
	defer fmt.Println("Leaving PutRank on ", sn.srvId)
	sn.ranksMapLock.Lock()
	defer sn.ranksMapLock.Unlock()
	key := args.Key
	sn.ranksMap[key] = args.Value
	fmt.Println("Rank Put for key", key, ": ", sn.ranksMap[key])
	return nil
}
