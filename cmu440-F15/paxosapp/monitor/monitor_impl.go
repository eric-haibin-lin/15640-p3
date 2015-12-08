package monitor

import (
	//	"fmt"
	"github.com/cmu440-F15/paxosapp/rpc/monitorrpc"
	"net"
	"net/http"
	"net/rpc"
	"time"
)

type monitorNode struct {
	masterHostPortMap  map[int]string
	slaveHostPortMap   map[int]string
	masterHeartBeatMap map[int]int
	slaveHeartBeatMap  map[int]int
	myHostPort         string
}

func NewMonitorNode(myHostPort string, masterHostPort, slaveHostPort []string) (MonitorNode, error) {
	var a monitorrpc.RemoteMonitorNode
	node := monitorNode{}
	node.masterHostPortMap = make(map[int]string)
	node.slaveHostPortMap = make(map[int]string)
	node.masterHeartBeatMap = make(map[int]int)
	node.slaveHeartBeatMap = make(map[int]int)
	node.myHostPort = myHostPort

	for id := 0; id < len(masterHostPort); id++ {
		node.masterHostPortMap[id] = masterHostPort[id]
	}
	for id := 0; id < len(slaveHostPort); id++ {
		node.slaveHostPortMap[id] = slaveHostPort[id]
	}

	listener, err := net.Listen("tcp", myHostPort)
	if err != nil {
		return nil, err
	}

	a = &node

	err = rpc.RegisterName("MonitorNode", monitorrpc.Wrap(a))
	if err != nil {
		return nil, err
	}

	rpc.HandleHTTP()
	go http.Serve(listener, nil)

	return a, nil
}

func (sn *monitorNode) HeartBeat(args *monitorrpc.HeartBeatArgs, reply *monitorrpc.HeartBeatReply) error {
	if args.Type == monitorrpc.Master {
		sn.masterHeartBeatMap[args.Id] += 1
	}
	if args.Type == monitorrpc.Slave {
		sn.slaveHeartBeatMap[args.Id] += 1
	}
	return nil
}

func (sn *monitorNode) CheckHealth() {
	time.Sleep(time.Second * 10)
	for {
		for index, _ := range sn.masterHostPortMap {
			_, ok := sn.masterHeartBeatMap[index]
			if !ok {
				//Master is down, replace with new master node

			} else {
				delete(sn.masterHeartBeatMap, index)
			}
		}

		for index, _ := range sn.masterHostPortMap {
			_, ok := sn.slaveHeartBeatMap[index]
			if !ok {
				//Slave is down, notify the master node
			} else {
				delete(sn.slaveHeartBeatMap, index)
			}
		}
		time.Sleep(time.Second * 4)
	}
}
