package paxos

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cmu440-F15/paxosapp/rpc/paxosrpc"
	"github.com/cmu440-F15/paxosapp/rpc/slaverpc"
	"math/rand"
	"net"
	"net/http"
	"net/rpc"
	"sync"
	"time"
)

const (
	NumCopies = 5
)

type paxosNode struct {
	myHostPort     string
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
func NewPaxosNode(myHostPort string, hostMap map[int]string, numNodes, srvId, numRetries int, replace bool, slaveMap map[int]string, numSlaves int) (PaxosNode, error) {
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
		node.valuesMap = f.(map[string][NumCopies]int)
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
	}

	// now try dialing all client nodes.. they should ideally already be up
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

	var targetSlaves [NumCopies]int
	//now select a group of slaves to store this value on

	i := 0
	rand.Seed(time.Now().UnixNano())
	for i < NumCopies {
		for {
			num := rand.Intn(pn.numSlaves)

			var found int
			found = 0
			//fmt.Println("The slice is ", targetSlaves)
			for _, val := range targetSlaves {
				//fmt.Println("j is ", j)
				if val == num {
					found = 1
					break
				}
			}
			if found == 0 {
				targetSlaves[i] = num
				break
			}
		}
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
		fmt.Println("Asking slave ", slaveId)
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
		fmt.Println("Asking slave ", slaveId)
		err := pn.slaveDialerMap[slaveId].Call("SlaveNode.Get", &getArgs, &getReply)
		if err == nil {
			reply.Value = getReply.Value
			fmt.Println("Slave ", slaveId, " has the data for ", args.Key, "!")
			return nil
		}
	}
	return errors.New("No slave replied.")
}

func (pn *paxosNode) GetAllLinks(args *paxosrpc.GetAllLinksArgs, reply *paxosrpc.GetAllLinksReply) error {
	reply.LinksMap = make(map[string][]string)
	for key, value := range pn.valuesMap {
		var getArgs slaverpc.GetArgs
		getArgs.Key = key

		var getReply slaverpc.GetReply
		for _, slaveId := range value {
			err := pn.slaveDialerMap[slaveId].Call("SlaveNode.Get", &getArgs, &getReply)
			if err == nil {
				reply.LinksMap[key] = getReply.Value
				break
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
		fmt.Println("This key already has some slaves. Calling append on them")

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

	var targetSlaves [NumCopies]int
	//now select a group of slaves to store this value on

	i := 0
	rand.Seed(time.Now().UnixNano())
	for i < NumCopies {
		for {
			num := rand.Intn(pn.numSlaves)

			var found int
			found = 0
			//fmt.Println("The slice is ", targetSlaves)
			for _, val := range targetSlaves {
				//fmt.Println("j is ", j)
				if val == num {
					found = 1
					break
				}
			}
			if found == 0 {
				targetSlaves[i] = num
				break
			}
		}
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

	for i := 0; i < pn.numNodes; i++ {
		ret, _ := <-preparechan
		if ret.timeout == 1 {
			fmt.Println("Didn't finish prepare stage even after 15 seconds")
			return errors.New("Didn't finish prepare stage even after 15 seconds")
		}
		if ret.prepReply.Status == paxosrpc.OK {
			okcount++
		}
		if ret.prepReply.N_a != 0 && ret.prepReply.N_a > max_n {
			max_n = ret.prepReply.N_a
			max_v = ret.prepReply.V_a
		}
		if okcount >= ((pn.numNodes / 2) + 1) {
			break
		}
	}

	if !(okcount >= ((pn.numNodes / 2) + 1)) {
		return errors.New("Didn't get a majority in prepare phase")
	}

	//var valueToPropose [NumCopies]int
	/*if max_n != 0 { //someone suggested a different value
		valueToPropose = max_v
	} else {
		valueToPropose = args.V
	}*/

	for k, v := range pn.hostMap {
		fmt.Println("Will call Accept on ", v)
		go accept(pn, k, max_v, args.Key, args.N, acceptchan)
	}

	okcount = 0

	for i := 0; i < pn.numNodes; i++ {
		ret, _ := <-acceptchan
		if ret.timeout == 1 {
			fmt.Println("Didn't finish accept stage even after 15 seconds")
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
			return errors.New("Didn't finish commit stage even after 15 seconds")
		}
	}

	//reply.V = max_v
	reply.Status = paxosrpc.OK
	reply.V = max_v

	fmt.Println(pn.myHostPort, " has successfully proposed the set ", max_v)
	return nil

	// so a group of 3 slaves has been selected to store the data
	// now actually store the data

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

/*
const (
	OK     Status = iota + 1 // Paxos replied OK
	Reject                   // Paxos rejected the message
)

const (
	KeyFound    Lookup = iota + 1 // GetValue key found
	KeyNotFound                   // GetValue key not found
)

type ProposalNumberArgs struct {
	Key string
}

type ProposalNumberReply struct {
	N int
}

type ProposeArgs struct {
	N   int // Proposal number
	Key string
	V   []string
}

type ProposeReply struct {
	Status Status
}

type PrepareArgs struct {
	Key string
	N   int
}

type PrepareReply struct {
	Status Status
	N_a    int            // Highest proposal number accepted
	V_a    [NumCopies]int // Corresponding value
}

type AcceptArgs struct {
	Key string
	N   int
	V   [NumCopies]int
}

type AcceptReply struct {
	Status Status
}

type CommitArgs struct {
	Key string
	V   [NumCopies]int
}

type CommitReply struct {
	// No content, no reply necessary
}

type ReplaceServerArgs struct {
	SrvID    int // Server being replaced
	Hostport string
}

type ReplaceServerReply struct {
	// No content necessary
}

type ReplaceCatchupArgs struct {
	// No content necessary
}

type ReplaceCatchupReply struct {
	Data []byte
}

type GetAllLinksArgs struct {
	// No content necessary, just get the whole damn thing lol
}

type GetAllLinksReply struct {
	LinksMap map[string][]string
}

type GetLinksArgs struct {
	Key string
}

type GetLinksReply struct {
	Value []string
}

type AppendArgs struct {
	Key   string
	Value []string
}

type AppendReply struct {
	//not sure what to keep here
}
*/
