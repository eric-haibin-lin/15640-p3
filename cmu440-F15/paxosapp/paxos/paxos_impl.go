package paxos

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cmu440-F15/paxosapp/common"
	"github.com/cmu440-F15/paxosapp/rpc/monitorrpc"
	"github.com/cmu440-F15/paxosapp/rpc/paxosrpc"
	"github.com/cmu440-F15/paxosapp/rpc/slaverpc"
	"net"
	"net/http"
	"net/rpc"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	NumCopies = 5
)

type paxosNode struct {
	myHostPort     string
	monitor        common.Conn
	numNodes       int
	numSlaves      int
	srvId          int
	hostMap        map[int]string
	slaveMap       map[int]string
	paxosDialerMap map[int]*rpc.Client
	slaveDialerMap map[int]*rpc.Client

	/* The main key-value store */
	valuesMap map[string][NumCopies]int

	/* Note - acceptedValuesMap and acceptedSeqNumMap should be used hand-in-hand */

	/* Temporary map which should be populated only when accept is called and
	should be cleared when commit is called */
	acceptedValuesMap map[string][NumCopies]int

	/* Temporary map which should be populated only when accept is called and
	should be cleared when commit is called */
	acceptedSeqNumMap map[string]int

	/* Again, a temporary map which should be populated when prepare is called */
	maxSeqNumSoFar map[string]int

	/* Next seqNum for particular key. Strictly increasing per key per node */
	nextSeqNumMap map[string]int

	maxSeqNumSoFarLock    *sync.Mutex
	valuesMapLock         *sync.Mutex
	acceptedValuesMapLock *sync.Mutex
	acceptedSeqNumMapLock *sync.Mutex
	nextSeqNumMapLock     *sync.Mutex
}

// NewPaxosNode creates a new PaxosNode. This function should return only when
// all nodes have joined the ring, and should return a non-nil error if the node
// could not be started in spite of dialing the other nodes numRetries times.
//
// hostMap is a map from node IDs to their hostports, numNodes is the number
// of nodes in the ring, replace is a flag which indicates whether this node
// is a replacement for a node which failed.
func NewPaxosNode(myHostPort, monitorHostPort string, hostMap map[int]string, numNodes, srvId, numRetries int, replace bool, slaveMap map[int]string, numSlaves int) (PaxosNode, error) {
	fmt.Println("myhostport is ", myHostPort, "Numnodes is ", numNodes, "srvid is ", srvId)

	var a paxosrpc.RemotePaxosNode

	node := paxosNode{}

	node.srvId = srvId
	node.numNodes = numNodes
	node.myHostPort = myHostPort
	node.hostMap = make(map[int]string)
	node.slaveMap = make(map[int]string)

	node.valuesMap = make(map[string][NumCopies]int)

	node.acceptedValuesMap = make(map[string][NumCopies]int)
	node.acceptedSeqNumMap = make(map[string]int)

	node.maxSeqNumSoFar = make(map[string]int)
	node.nextSeqNumMap = make(map[string]int)

	node.paxosDialerMap = make(map[int]*rpc.Client)
	node.slaveDialerMap = make(map[int]*rpc.Client)

	node.numSlaves = numSlaves

	node.valuesMapLock = &sync.Mutex{}
	node.acceptedValuesMapLock = &sync.Mutex{}
	node.acceptedSeqNumMapLock = &sync.Mutex{}
	node.maxSeqNumSoFarLock = &sync.Mutex{}
	node.nextSeqNumMapLock = &sync.Mutex{}

	for k, v := range hostMap {
		node.hostMap[k] = v
	}

	for k, v := range slaveMap {
		node.slaveMap[k] = v
	}

	listener, err := net.Listen("tcp", myHostPort)
	if err != nil {
		return nil, err
	}

	a = &node

	err = rpc.RegisterName("PaxosNode", paxosrpc.Wrap(a))
	if err != nil {
		return nil, err
	}

	rpc.HandleHTTP()
	go http.Serve(listener, nil)

	for key, v := range hostMap {
		dialer, err := rpc.DialHTTP("tcp", v)

		cntr := 0

		if err != nil {
			for {
				fmt.Println(myHostPort, " couldn't dial", v, ". Trying again.")

				cntr = cntr + 1

				if cntr == numRetries {
					fmt.Println("Couldn't connect even after all retries.", myHostPort, " aborting.")
					return nil, errors.New("Couldn't dial a node")
				}
				time.Sleep(1 * time.Second)
				dialer, err := rpc.DialHTTP("tcp", v)
				if err == nil {
					node.paxosDialerMap[key] = dialer
					break
				}
			}
		} else {
			node.paxosDialerMap[key] = dialer
		}

		fmt.Println(myHostPort, " dialed fellow paxosnode", v, " successfully")
	}

	//get updated values map when it's a node for replacement
	if replace {
		nextSrv := ""
		var k int
		var v string
		for k, v = range hostMap {
			//use the first entry in hostMap to retrieve the values map. do not send the request to the new node itself
			if v != myHostPort {
				nextSrv = v
				break
			}
		}
		args := paxosrpc.ReplaceCatchupArgs{}
		reply := paxosrpc.ReplaceCatchupReply{}
		//dialer, err := rpc.DialHTTP("tcp", nextSrv)
		if err != nil {
			fmt.Println(myHostPort, " couldn't dial", nextSrv, " (for ReplaceCatchup)")
			return nil, err
		}
		err = node.paxosDialerMap[k].Call("PaxosNode.RecvReplaceCatchup", &args, &reply)
		if err != nil {
			fmt.Println("ERROR: Couldn't Dial RecvReplaceCatchup on ", nextSrv)
		}

		var f interface{}
		json.Unmarshal(reply.Data, &f)
		node.valuesMapLock.Lock()
		//Convert []interface{} to [numCopies]int by iterating the interface{} slice
		for key, arr := range f.(map[string]interface{}) {
			slices := arr.([]interface{})
			var copies [NumCopies]int
			for index, value := range slices {
				fmt.Println(index, int(value.(float64)))
				copies[index] = int(value.(float64))
			}
			node.valuesMap[key] = copies
		}
		node.valuesMapLock.Unlock()

		fmt.Println("Received values from peers. The value map has the following entries:")
		for k, v := range node.valuesMap {
			fmt.Println(k, v)
		}

		//now call RecvReplaceServer on each of the other nodes to inform them that
		//I am now taking the place of the failed node

		for k, v := range hostMap {
			if v != myHostPort {
				//dialer, err := rpc.DialHTTP("tcp", v)
				if err != nil {
					fmt.Println(myHostPort, " couldn't dial", nextSrv, " (for RecvReplaceServer)")
					return nil, err
				}
				args := paxosrpc.ReplaceServerArgs{}
				reply := paxosrpc.ReplaceServerReply{}

				args.SrvID = srvId
				args.Hostport = myHostPort
				err = node.paxosDialerMap[k].Call("PaxosNode.RecvReplaceServer", &args, &reply)
				if err != nil {
					fmt.Println("ERROR: Couldn't Dial RecvReplaceServer on ", nextSrv)
				}
			}
		}
	} else {
		// this node has been generated for the first time
		// set the sizes of all nodes to 0
		for i := 0; i < numSlaves; i++ {
			sizeKey := strconv.Itoa(i)
			sizeKey = sizeKey + ":size"
			var sizeSlice [NumCopies]int
			sizeSlice[0] = 0
			node.valuesMap[sizeKey] = sizeSlice
		}
	}

	// now try dialing all slave nodes.. they should ideally already be up
	for k, v := range node.slaveMap {
		dialer, err := rpc.DialHTTP("tcp", v)

		cntr := 0

		if err != nil {
			for {
				fmt.Println(myHostPort, " couldn't dial slave", v, ". Trying again.")

				cntr = cntr + 1
				if cntr == numRetries {
					fmt.Println("Couldn't connect even after all retries.", myHostPort, " aborting.")
					return nil, errors.New("Couldn't dial a node")
				}
				time.Sleep(1 * time.Second)
				dialer, err := rpc.DialHTTP("tcp", v)
				if err == nil {
					node.slaveDialerMap[k] = dialer
					break
				}
			}
		} else {
			node.slaveDialerMap[k] = dialer
		}

		fmt.Println(myHostPort, " dialed slave ", v, " successfully")
	}

	go (&node).HeartBeat(monitorHostPort)

	return a, nil
}

func (pn *paxosNode) GetNextProposalNumber(args *paxosrpc.ProposalNumberArgs, reply *paxosrpc.ProposalNumberReply) error {
	fmt.Println("GetNextProposalNumber invoked on ", pn.srvId)
	key := args.Key
	// increase the nextNum for this key, append distinct srvId
	pn.nextSeqNumMapLock.Lock()
	defer pn.nextSeqNumMapLock.Unlock()
	pn.nextSeqNumMap[key] += 1
	nextNum := pn.nextSeqNumMap[key]
	nextNum = nextNum*1000 + pn.srvId
	reply.N = nextNum
	return nil
}

type prepReplyAndTimeout struct {
	prepReply paxosrpc.PrepareReply
	timeout   int
}

type accReplyAndTimeout struct {
	accReply paxosrpc.AcceptReply
	timeout  int
}

func prepare(pn *paxosNode, srvId int, key string, seqnum int, preparechan chan prepReplyAndTimeout) {
	args := paxosrpc.PrepareArgs{}
	args.Key = key
	args.N = seqnum

	reply := paxosrpc.PrepareReply{}

	err := pn.paxosDialerMap[srvId].Call("PaxosNode.RecvPrepare", &args, &reply)

	if err != nil {
		fmt.Println("RPC RecvPrepare failed!")
		return
	}

	var ret prepReplyAndTimeout
	ret.prepReply = reply
	ret.timeout = 0
	fmt.Println("Got Prepare reply from ", pn.hostMap[srvId], ". The N is ", reply.N_a, " and the value is ", reply.V_a)
	preparechan <- ret
}

func accept(pn *paxosNode, srvId int, value [NumCopies]int, key string, seqnum int, acceptchan chan accReplyAndTimeout) {
	args := paxosrpc.AcceptArgs{}
	args.Key = key
	args.N = seqnum
	args.V = value

	reply := paxosrpc.AcceptReply{}

	err := pn.paxosDialerMap[srvId].Call("PaxosNode.RecvAccept", &args, &reply)

	if err != nil {
		fmt.Println("RPC RecvAccept failed!")
		return
	}

	var ret accReplyAndTimeout
	ret.accReply = reply
	ret.timeout = 0
	fmt.Println("Got Accept reply from ", pn.hostMap[srvId], ". The Status is ", reply.Status)
	acceptchan <- ret
}

func commit(pn *paxosNode, srvId int, value [NumCopies]int, key string, commitchan chan int) {
	args := paxosrpc.CommitArgs{}
	args.Key = key
	args.V = value

	reply := paxosrpc.CommitReply{}

	err := pn.paxosDialerMap[srvId].Call("PaxosNode.RecvCommit", &args, &reply)

	if err != nil {
		fmt.Println("RPC RecvCommit failed!")
		return
	}

	fmt.Println("Got Commit reply from ", pn.hostMap[srvId])
	commitchan <- 0
}

func wakeMeUpAfter15Seconds(preparechan chan prepReplyAndTimeout, acceptchan chan accReplyAndTimeout,
	commitchan chan int) {
	time.Sleep(15 * time.Second)

	var ret1 prepReplyAndTimeout
	ret1.timeout = 1
	preparechan <- ret1

	var ret2 accReplyAndTimeout
	ret2.timeout = 1
	acceptchan <- ret2

	commitchan <- 1

}

type idAndSize struct {
	id   int
	size int
}

func (pn *paxosNode) PutRank(args *paxosrpc.PutRankArgs, reply *paxosrpc.PutRankReply) error {
	fmt.Println("PutRank called on ", pn.myHostPort, " with key = ", args.Key)
	var putargs slaverpc.PutRankArgs
	putargs.Key = args.Key
	putargs.Value = args.Value

	var putreply slaverpc.PutRankReply
	slaveList, ok := pn.valuesMap[args.Key]

	if ok {
		fmt.Println("This key already has some slaves. Calling PutRank on them")

		for _, slaveId := range slaveList {
			err := pn.slaveDialerMap[slaveId].Call("SlaveNode.PutRank", &putargs, &putreply)
			if err != nil {
				fmt.Println("PutRank RPC Failed on ", pn.slaveMap[slaveId])
			}
		}
		return nil
	}

	var propNumArgs paxosrpc.ProposalNumberArgs
	var propNumReply paxosrpc.ProposalNumberReply
	propNumArgs.Key = args.Key

	pn.GetNextProposalNumber(&propNumArgs, &propNumReply)

	fmt.Println("Got proposal number as ", propNumReply.N)

	var idAndSizeList []idAndSize
	//now select a group of slaves to store this value on
	for i := 0; i < pn.numSlaves; i++ {
		key := strconv.Itoa(i)
		key = key + ":size"
		size := (pn.valuesMap[key])[0]

		ias := idAndSize{id: i, size: size}
		idAndSizeList = append(idAndSizeList, ias)
	}

	sort.Sort(SlaveSlice(idAndSizeList))
	i := 0

	var targetSlaves [NumCopies]int
	for i < NumCopies {
		targetSlaves[i] = idAndSizeList[i].id
		i += 1
	}

	fmt.Println("Generated the targetSlaves slice as ", targetSlaves)

	var proposeArgs paxosrpc.ProposeArgs
	var proposeReply paxosrpc.ProposeReply

	proposeArgs.N = propNumReply.N
	proposeArgs.Key = args.Key
	proposeArgs.V = targetSlaves

	fmt.Println(pn.myHostPort, " will now propose for key = ", proposeArgs.Key, " and value = ", proposeArgs.V)

	err := pn.Propose(&proposeArgs, &proposeReply)
	if err != nil {
		fmt.Println("Propose failed!")
		return errors.New("Propose failed!")
	}

	fmt.Println("Propose succeeded and the value committed was ", proposeReply.V)

	for _, slaveId := range proposeReply.V {
		err := pn.slaveDialerMap[slaveId].Call("SlaveNode.PutRank", &putargs, &putreply)
		if err != nil {
			fmt.Println("PutRank RPC Failed on ", pn.slaveMap[slaveId])
		}
	}

	return nil
}

func (pn *paxosNode) GetRank(args *paxosrpc.GetRankArgs, reply *paxosrpc.GetRankReply) error {
	fmt.Println("GetRank invoked on Node ", pn.srvId, " for key ", args.Key)

	_, ok := pn.valuesMap[args.Key]

	if !ok {
		fmt.Println("Key not found")
		return errors.New("Key not found")
	}

	for _, slaveId := range pn.valuesMap[args.Key] {
		var getRankArgs slaverpc.GetRankArgs
		getRankArgs.Key = args.Key

		var getRankReply slaverpc.GetRankReply
		//fmt.Println("Asking slave ", slaveId)
		err := pn.slaveDialerMap[slaveId].Call("SlaveNode.GetRank", &getRankArgs, &getRankReply)
		if err == nil {
			reply.Value = getRankReply.Value
			fmt.Println("Slave ", slaveId, " has the data for ", args.Key, "!")
			return nil
		}
	}
	return errors.New("No slave replied.")
}

func (pn *paxosNode) GetLinks(args *paxosrpc.GetLinksArgs, reply *paxosrpc.GetLinksReply) error {
	fmt.Println("GetLinks invoked on Node ", pn.srvId, " for key ", args.Key)
	list, ok := pn.valuesMap[args.Key]
	if !ok {
		fmt.Println("Key", args.Key, "not found")
		return errors.New("Key " + args.Key + " not found")
	}
	for _, slaveId := range list {
		var getArgs slaverpc.GetArgs
		getArgs.Key = args.Key

		var getReply slaverpc.GetReply
		//fmt.Println("Asking slave ", slaveId)
		err := pn.slaveDialerMap[slaveId].Call("SlaveNode.Get", &getArgs, &getReply)
		if err == nil && getReply.Status == 1 {
			reply.Value = getReply.Value
			fmt.Println("Slave ", slaveId, " has the data for ", args.Key, "!")
			return nil
		}
	}
	return errors.New("No slave replied.")
}

func (pn *paxosNode) GetAllLinks(args *paxosrpc.GetAllLinksArgs, reply *paxosrpc.GetAllLinksReply) error {
	fmt.Println("GetAllLinks invoked on Node ", pn.srvId)
	reply.LinksMap = make(map[string][]string)
	for key, value := range pn.valuesMap {
		var getArgs slaverpc.GetArgs
		getArgs.Key = key

		var getReply slaverpc.GetReply
		if !strings.HasSuffix(key, ":size") {
			for _, slaveId := range value {
				//fmt.Println("Asking slave ", slaveId)
				err := pn.slaveDialerMap[slaveId].Call("SlaveNode.Get", &getArgs, &getReply)
				if err == nil && getReply.Status == 1 {
					reply.LinksMap[key] = getReply.Value
					break
				}
			}
		}
	}
	return nil
}

func (pn *paxosNode) updateSizes(url string, value []string) error {
	sliceSize := 0

	for _, str := range value {
		sliceSize += len(str)
	}

	var propNumArgs paxosrpc.ProposalNumberArgs
	var propNumReply paxosrpc.ProposalNumberReply

	for _, i := range pn.valuesMap[url] {
		key := strconv.Itoa(i)
		key = key + ":size"

		propNumArgs.Key = key

		for {
			pn.GetNextProposalNumber(&propNumArgs, &propNumReply)

			fmt.Println("For updating size, got next proposal number as ", propNumReply.N)

			var proposeArgs paxosrpc.ProposeArgs
			var proposeReply paxosrpc.ProposeReply

			var newSizeSlice [NumCopies]int
			newSizeSlice[0] = (pn.valuesMap[key])[0] + sliceSize

			proposeArgs.N = propNumReply.N
			proposeArgs.Key = key
			proposeArgs.V = newSizeSlice

			fmt.Println(pn.myHostPort, " will now propose for key = ", proposeArgs.Key, " and value = ", proposeArgs.V)

			pn.Propose(&proposeArgs, &proposeReply)

			if proposeReply.Status == paxosrpc.OK {
				fmt.Println("Propose succeeded and the value committed was ", proposeReply.V)
				break
			} else if proposeReply.Status == paxosrpc.OtherValueCommitted {
				fmt.Println("Some other value was committed! Retry")
			} else {
				fmt.Println("Paxos rejected! Retry")
			}
		}
	}

	return nil
}

func (pn *paxosNode) Append(args *paxosrpc.AppendArgs, reply *paxosrpc.AppendReply) error {

	fmt.Println("Append called on ", pn.myHostPort, " with key = ", args.Key)
	var appendArgs slaverpc.AppendArgs
	appendArgs.Key = args.Key
	appendArgs.Value = args.Value

	var appendReply slaverpc.AppendReply
	slaveList, ok := pn.valuesMap[args.Key]

	if ok {
		fmt.Println("This key already has some slaves. First update the sizes.")

		pn.updateSizes(args.Key, args.Value)

		fmt.Println("Size updation done! Now call Append on all")

		for _, slaveId := range slaveList {
			err := pn.slaveDialerMap[slaveId].Call("SlaveNode.Append", &appendArgs, &appendReply)
			if err != nil {
				fmt.Println("Append RPC Failed on ", pn.slaveMap[slaveId])
			}
		}
		return nil
	}

	var propNumArgs paxosrpc.ProposalNumberArgs
	var propNumReply paxosrpc.ProposalNumberReply
	propNumArgs.Key = args.Key

	pn.GetNextProposalNumber(&propNumArgs, &propNumReply)

	fmt.Println("Got proposal number as ", propNumReply.N)

	//now select a group of slaves to store this value on

	var idAndSizeList []idAndSize
	//now select a group of slaves to store this value on
	for i := 0; i < pn.numSlaves; i++ {
		key := strconv.Itoa(i)
		key = key + ":size"
		size := (pn.valuesMap[key])[0]

		ias := idAndSize{id: i, size: size}
		idAndSizeList = append(idAndSizeList, ias)
	}

	sort.Sort(SlaveSlice(idAndSizeList))
	i := 0

	var targetSlaves [NumCopies]int
	for i < NumCopies {
		targetSlaves[i] = idAndSizeList[i].id
		i += 1
	}

	fmt.Println("Generated the targetSlaves slice as ", targetSlaves)

	var proposeArgs paxosrpc.ProposeArgs
	var proposeReply paxosrpc.ProposeReply

	proposeArgs.N = propNumReply.N
	proposeArgs.Key = args.Key
	proposeArgs.V = targetSlaves

	fmt.Println(pn.myHostPort, " will now propose for key = ", proposeArgs.Key, " and value = ", proposeArgs.V)

	err := pn.Propose(&proposeArgs, &proposeReply)
	if err != nil {
		fmt.Println("Propose failed!")
		return errors.New("Propose failed!")
	}

	fmt.Println("Propose succeeded and the value committed was ", proposeReply.V)

	fmt.Println("Now will try to propose size values")

	pn.updateSizes(args.Key, args.Value)

	fmt.Println("Size updation done! Now call Append on all appropriate nodes")
	for _, slaveId := range proposeReply.V {
		err := pn.slaveDialerMap[slaveId].Call("SlaveNode.Append", &appendArgs, &appendReply)
		if err != nil {
			fmt.Println("Append RPC Failed on ", pn.slaveMap[slaveId])
		}
	}

	reply.Status = paxosrpc.OK
	return nil

}

func (pn *paxosNode) Propose(args *paxosrpc.ProposeArgs, reply *paxosrpc.ProposeReply) error {
	preparechan := make(chan prepReplyAndTimeout, 100)
	acceptchan := make(chan accReplyAndTimeout, 100)
	commitchan := make(chan int, 100)

	fmt.Println("In Propose of ", pn.srvId)

	fmt.Println("Key is ", args.Key, ", V is ", args.V, " and N is ", args.N)

	go wakeMeUpAfter15Seconds(preparechan, acceptchan, commitchan)

	for k, v := range pn.hostMap {
		fmt.Println("Will call Prepare on ", v)
		go prepare(pn, k, args.Key, args.N, preparechan)
	}

	okcount := 0

	max_n := 0
	var max_v [NumCopies]int
	max_v = args.V
	//max_v = args.V

	reply.Status = paxosrpc.OK
	for i := 0; i < pn.numNodes; i++ {
		ret, _ := <-preparechan
		if ret.timeout == 1 {
			fmt.Println("Didn't finish prepare stage even after 15 seconds")
			reply.Status = paxosrpc.Reject
			return errors.New("Didn't finish prepare stage even after 15 seconds")
		}
		if ret.prepReply.Status == paxosrpc.OK {
			okcount++
		}
		if ret.prepReply.N_a != 0 && ret.prepReply.N_a > max_n {
			max_n = ret.prepReply.N_a
			max_v = ret.prepReply.V_a
			reply.Status = paxosrpc.OtherValueCommitted
		}
		if okcount >= ((pn.numNodes / 2) + 1) {
			break
		}
	}

	if !(okcount >= ((pn.numNodes / 2) + 1)) {
		reply.Status = paxosrpc.Reject
		return errors.New("Didn't get a majority in prepare phase")
	}

	for k, v := range pn.hostMap {
		fmt.Println("Will call Accept on ", v)
		go accept(pn, k, max_v, args.Key, args.N, acceptchan)
	}

	okcount = 0

	for i := 0; i < pn.numNodes; i++ {
		ret, _ := <-acceptchan
		if ret.timeout == 1 {
			fmt.Println("Didn't finish accept stage even after 15 seconds")
			reply.Status = paxosrpc.Reject
			return errors.New("Didn't finish accept stage even after 15 seconds")
		}
		if ret.accReply.Status == paxosrpc.OK {
			okcount++
		}
		if okcount >= ((pn.numNodes / 2) + 1) {
			break
		}
	}

	if !(okcount >= ((pn.numNodes / 2) + 1)) {
		reply.Status = paxosrpc.Reject
		return errors.New("Didn't get a majority in accept phase")
	}

	okcount = 0
	for k, v := range pn.hostMap {
		fmt.Println("Will call Commit on ", v)
		go commit(pn, k, max_v, args.Key, commitchan)
	}

	for i := 0; i < pn.numNodes; i++ {
		_, ok := <-commitchan
		if !ok {
			fmt.Println("Didn't finish commit stage even after 15 seconds")
			reply.Status = paxosrpc.Reject
			return errors.New("Didn't finish commit stage even after 15 seconds")
		}
	}

	//reply.V = max_v
	reply.V = max_v

	fmt.Println(pn.myHostPort, " has successfully proposed the set ", max_v)
	return nil
}

/*func (pn *paxosNode) GetValue(args *paxosrpc.GetValueArgs, reply *paxosrpc.GetValueReply) error {
	fmt.Println("Inside GetValue of ", pn.myHostPort)
	defer fmt.Println("Leaving GetValue of ", pn.myHostPort)
	pn.valuesMapLock.Lock()
	val, ok := pn.valuesMap[args.Key]
	pn.valuesMapLock.Unlock()

	if ok {
		reply.V = val
		reply.Status = paxosrpc.KeyFound
		return nil
	}

	reply.Status = paxosrpc.KeyNotFound
	return nil
}*/

func (pn *paxosNode) RecvPrepare(args *paxosrpc.PrepareArgs, reply *paxosrpc.PrepareReply) error {
	defer fmt.Println("Leaving RecvPrepare of ", pn.myHostPort)
	fmt.Println("In RecvPrepare of ", pn.myHostPort)
	key := args.Key
	num := args.N

	pn.maxSeqNumSoFarLock.Lock()
	maxNum := pn.maxSeqNumSoFar[key]
	pn.maxSeqNumSoFarLock.Unlock()

	pn.acceptedValuesMapLock.Lock()
	val, ok := pn.acceptedValuesMap[key]
	pn.acceptedValuesMapLock.Unlock()

	pn.acceptedSeqNumMapLock.Lock()
	seqNum := pn.acceptedSeqNumMap[key]
	pn.acceptedSeqNumMapLock.Unlock()

	if !ok {
		reply.N_a = -1
	} else {
		reply.V_a = val
		reply.N_a = seqNum
	}

	// reject proposal when its proposal number is not higher than the highest number it's ever seen
	if maxNum > num {
		fmt.Println("In RecvPrepare of ", pn.myHostPort, "rejected proposal:", key, num, "maxNum:", maxNum)
		//return -1 for proposal promised
		reply.Status = paxosrpc.Reject

		return nil
	}
	// promise proposal when its higher. return with the number and value accepted
	fmt.Println("In RecvPrepare of ", pn.myHostPort, "accepted proposal:", key, num, "maxNum:", maxNum)

	pn.maxSeqNumSoFarLock.Lock()
	pn.maxSeqNumSoFar[key] = num
	pn.maxSeqNumSoFarLock.Unlock()

	// fill reply with accepted seqNum and value. default is -1
	reply.Status = paxosrpc.OK
	return nil
}

func (pn *paxosNode) RecvAccept(args *paxosrpc.AcceptArgs, reply *paxosrpc.AcceptReply) error {
	defer fmt.Println("Leaving RecvAccept of ", pn.myHostPort)
	fmt.Println("In RecvAccept of ", pn.myHostPort)
	key := args.Key
	num := args.N
	value := args.V

	pn.maxSeqNumSoFarLock.Lock()
	maxNum := pn.maxSeqNumSoFar[key]
	pn.maxSeqNumSoFarLock.Unlock()

	// reject proposal when its proposal number is not higher than the highest number it's ever seen
	if maxNum > num {
		fmt.Println("In RecvAccept of ", pn.myHostPort, "rejected proposal:", key, num, value, "maxNum:", maxNum)
		reply.Status = paxosrpc.Reject
		return nil
	}
	// accept proposal when its higher. update with the number and value accepted
	fmt.Println("In RecvAccept of ", pn.myHostPort, "accepted proposal:", key, num, value)

	pn.maxSeqNumSoFarLock.Lock()
	pn.maxSeqNumSoFar[key] = num
	pn.maxSeqNumSoFarLock.Unlock()

	pn.acceptedValuesMapLock.Lock()
	pn.acceptedValuesMap[key] = value
	pn.acceptedValuesMapLock.Unlock()

	pn.acceptedSeqNumMapLock.Lock()
	pn.acceptedSeqNumMap[key] = num
	pn.acceptedSeqNumMapLock.Unlock()

	reply.Status = paxosrpc.OK
	return nil
}

func (pn *paxosNode) RecvCommit(args *paxosrpc.CommitArgs, reply *paxosrpc.CommitReply) error {
	defer fmt.Println("Leaving RecvCommit of ", pn.myHostPort)
	key := args.Key
	value := args.V

	// update the value and clear the map for accepted value and number
	fmt.Println("In RecvCommit of ", pn.myHostPort, "committing:", key, value)
	pn.valuesMapLock.Lock()
	pn.valuesMap[key] = value
	pn.valuesMapLock.Unlock()

	pn.acceptedValuesMapLock.Lock()
	delete(pn.acceptedValuesMap, key)
	pn.acceptedValuesMapLock.Unlock()

	pn.acceptedSeqNumMapLock.Lock()
	delete(pn.acceptedSeqNumMap, key)
	pn.acceptedSeqNumMapLock.Unlock()
	return nil
}

func (pn *paxosNode) RecvReplaceServer(args *paxosrpc.ReplaceServerArgs, reply *paxosrpc.ReplaceServerReply) error {
	fmt.Println("In ReplaceServer of ", pn.myHostPort)
	fmt.Println("Some node is replacing the node with ID : ", args.SrvID)
	fmt.Println("Its hostport is ", args.Hostport)

	dialer, _ := rpc.DialHTTP("tcp", args.Hostport)

	pn.hostMap[args.SrvID] = args.Hostport
	pn.paxosDialerMap[args.SrvID] = dialer
	return nil
}

func (pn *paxosNode) RecvReplaceCatchup(args *paxosrpc.ReplaceCatchupArgs, reply *paxosrpc.ReplaceCatchupReply) error {
	fmt.Println("In RecvReplaceCatchup of ", pn.myHostPort)

	pn.valuesMapLock.Lock()
	fmt.Println("Marshalling", len(pn.valuesMap), "values for RecvReplaceCatchup")
	marshaledMap, err := json.Marshal(pn.valuesMap)
	pn.valuesMapLock.Unlock()

	if err != nil {
		fmt.Println("Failed to marshall", err)
		return err
	}
	reply.Data = marshaledMap
	return nil
}

func (pn *paxosNode) HeartBeat(hostPort string) {
	fmt.Println("Heartbeat invoked on Master node", pn.srvId)
	for {
		time.Sleep(time.Second * 2)
		fmt.Println("Try dialing Monitor node from Master", pn.srvId)
		var monitor common.Conn
		hostPorts := make([]string, 0)
		hostPorts = append(hostPorts, hostPort)
		monitorDialer, err := rpc.DialHTTP("tcp", hostPort)
		monitor.HostPort = hostPorts
		monitor.Dialer = monitorDialer
		if err != nil {
			fmt.Println(err)
		} else {
			pn.monitor = monitor
			break
		}
	}

	for {
		var args monitorrpc.HeartBeatArgs
		var reply monitorrpc.HeartBeatReply
		args.Id = pn.srvId
		args.Type = monitorrpc.Master
		//fmt.Println("Calling MonitorNode.HeartBeat to monitor node")
		err := pn.monitor.Dialer.Call("MonitorNode.HeartBeat", &args, &reply)
		if err != nil {
			fmt.Println(err)
		}
		time.Sleep(time.Second * 3)
	}
}

type SlaveSlice []idAndSize

func (slice SlaveSlice) Len() int {
	return len(slice)
}

func (slice SlaveSlice) Less(i, j int) bool {
	return slice[i].size < slice[j].size
}

func (slice SlaveSlice) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}
