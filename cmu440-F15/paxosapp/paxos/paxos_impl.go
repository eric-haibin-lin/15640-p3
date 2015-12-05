package paxos

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cmu440-F15/paxosapp/rpc/paxosrpc"
	"net"
	"net/http"
	"net/rpc"
	"sync"
	"time"
)

type paxosNode struct {
	myHostPort string
	numNodes   int
	srvId      int
	hostMap    map[int]string

	/* The main key-value store */
	valuesMap map[string]interface{}

	/* Note - acceptedValuesMap and acceptedSeqNumMap should be used hand-in-hand */

	/* Temporary map which should be populated only when accept is called and
	should be cleared when commit is called */
	acceptedValuesMap map[string]interface{}

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
func NewPaxosNode(myHostPort string, hostMap map[int]string, numNodes, srvId, numRetries int, replace bool) (PaxosNode, error) {
	fmt.Println("myhostport is ", myHostPort, "Numnodes is ", numNodes, "srvid is ", srvId)

	var a paxosrpc.RemotePaxosNode

	node := paxosNode{}

	node.srvId = srvId
	node.numNodes = numNodes
	node.myHostPort = myHostPort
	node.hostMap = make(map[int]string)

	node.valuesMap = make(map[string]interface{})

	node.acceptedValuesMap = make(map[string]interface{})
	node.acceptedSeqNumMap = make(map[string]int)

	node.maxSeqNumSoFar = make(map[string]int)
	node.nextSeqNumMap = make(map[string]int)

	node.valuesMapLock = &sync.Mutex{}
	node.acceptedValuesMapLock = &sync.Mutex{}
	node.acceptedSeqNumMapLock = &sync.Mutex{}
	node.maxSeqNumSoFarLock = &sync.Mutex{}
	node.nextSeqNumMapLock = &sync.Mutex{}

	for k, v := range hostMap {
		node.hostMap[k] = v
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

	for _, v := range hostMap {
		_, err := rpc.DialHTTP("tcp", v)

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
				_, err := rpc.DialHTTP("tcp", v)
				if err == nil {
					break
				}
			}
		}
		fmt.Println(myHostPort, " dialed ", v, " successfully")
	}

	//get updated values map when it's a node for replacement
	if replace {
		nextSrv := ""
		for _, v := range hostMap {
			//use the first entry in hostMap to retrieve the values map. do not send the request to the new node itself
			if v != myHostPort {
				nextSrv = v
				break
			}
		}
		args := paxosrpc.ReplaceCatchupArgs{}
		reply := paxosrpc.ReplaceCatchupReply{}
		dialer, err := rpc.DialHTTP("tcp", nextSrv)
		if err != nil {
			fmt.Println(myHostPort, " couldn't dial", nextSrv, " (for ReplaceCatchup)")
			return nil, err
		}
		err = dialer.Call("PaxosNode.RecvReplaceCatchup", &args, &reply)
		if err != nil {
			fmt.Println("ERROR: Couldn't Dial RecvReplaceCatchup on ", nextSrv)
		}

		var f interface{}
		json.Unmarshal(reply.Data, &f)
		node.valuesMapLock.Lock()
		node.valuesMap = f.(map[string]interface{})
		node.valuesMapLock.Unlock()

		fmt.Println("Received values from peers. The value map has the following entries:")
		for k, v := range node.valuesMap {
			fmt.Println(k, v)
		}

		//now call RecvReplaceServer on each of the other nodes to inform them that
		//I am now taking the place of the failed node

		for _, v := range hostMap {
			if v != myHostPort {
				dialer, err := rpc.DialHTTP("tcp", v)
				if err != nil {
					fmt.Println(myHostPort, " couldn't dial", nextSrv, " (for RecvReplaceServer)")
					return nil, err
				}
				args := paxosrpc.ReplaceServerArgs{}
				reply := paxosrpc.ReplaceServerReply{}

				args.SrvID = srvId
				args.Hostport = myHostPort
				err = dialer.Call("PaxosNode.RecvReplaceServer", &args, &reply)
				if err != nil {
					fmt.Println("ERROR: Couldn't Dial RecvReplaceServer on ", nextSrv)
				}
			}
		}
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

func prepare(pn *paxosNode, hostport string, key string, seqnum int, preparechan chan paxosrpc.PrepareReply) {
	dialer, err := rpc.DialHTTP("tcp", hostport)

	if err != nil {
		fmt.Println("ERROR: Couldn't Dial prepare on ", hostport)
		return
	}

	args := paxosrpc.PrepareArgs{}
	args.Key = key
	args.N = seqnum

	reply := paxosrpc.PrepareReply{}

	err = dialer.Call("PaxosNode.RecvPrepare", &args, &reply)

	if err != nil {
		fmt.Println("RPC RecvPrepare failed!")
		return
	}

	fmt.Println("Got Prepare reply from ", hostport)
	preparechan <- reply
}

func accept(pn *paxosNode, hostport string, value interface{}, key string, seqnum int, acceptchan chan paxosrpc.AcceptReply) {
	dialer, err := rpc.DialHTTP("tcp", hostport)

	if err != nil {
		fmt.Println("ERROR: Couldn't Dial accept on ", hostport)
		return
	}

	args := paxosrpc.AcceptArgs{}
	args.Key = key
	args.N = seqnum
	args.V = value

	reply := paxosrpc.AcceptReply{}

	err = dialer.Call("PaxosNode.RecvAccept", &args, &reply)

	if err != nil {
		fmt.Println("RPC RecvAccept failed!")
		return
	}

	acceptchan <- reply
}

func commit(pn *paxosNode, hostport string, value interface{}, key string, commitchan chan int) {
	dialer, err := rpc.DialHTTP("tcp", hostport)

	if err != nil {
		fmt.Println("ERROR: Couldn't Dial commit on ", hostport)
		return
	}

	args := paxosrpc.CommitArgs{}
	args.Key = key
	args.V = value

	reply := paxosrpc.CommitReply{}

	err = dialer.Call("PaxosNode.RecvCommit", &args, &reply)

	if err != nil {
		fmt.Println("RPC RecvCommit failed!")
		return
	}

	commitchan <- 1
}

func wakeMeUpAfter15Seconds(preparechan chan paxosrpc.PrepareReply, acceptchan chan paxosrpc.AcceptReply,
	commitchan chan int) {
	time.Sleep(15 * time.Second)

	close(preparechan)
	close(acceptchan)
	close(commitchan)
}
func (pn *paxosNode) Propose(args *paxosrpc.ProposeArgs, reply *paxosrpc.ProposeReply) error {
	preparechan := make(chan paxosrpc.PrepareReply, 100)
	acceptchan := make(chan paxosrpc.AcceptReply, 100)
	commitchan := make(chan int, 100)

	fmt.Println("In Propose of ", pn.srvId)

	fmt.Println("Key is ", args.Key, ", V is ", args.V, " and N is ", args.N)

	go wakeMeUpAfter15Seconds(preparechan, acceptchan, commitchan)

	for _, v := range pn.hostMap {
		fmt.Println("Will call Prepare on ", v)
		go prepare(pn, v, args.Key, args.N, preparechan)
	}

	okcount := 0

	max_n := 0
	max_v := args.V

	for i := 0; i < pn.numNodes; i++ {
		ret, ok := <-preparechan
		if !ok {
			fmt.Println("Didn't finish prepare stage even after 15 seconds")
			return errors.New("Didn't finish prepare stage even after 15 seconds")
		}
		if ret.Status == paxosrpc.OK {
			okcount++
			if ret.N_a != 0 && ret.N_a > max_n {
				max_n = ret.N_a
				max_v = ret.V_a
			}
			if okcount >= ((pn.numNodes / 2) + 1) {
				break
			}
		}
	}

	if !(okcount >= ((pn.numNodes / 2) + 1)) {
		return errors.New("Didn't get a majority in prepare phase")
	}

	var valueToPropose interface{}
	if max_n != 0 { //someone suggested a different value
		valueToPropose = args.V
	} else {
		valueToPropose = max_v
	}

	for _, v := range pn.hostMap {
		fmt.Println("Will call Accept on ", v)
		go accept(pn, v, valueToPropose, args.Key, args.N, acceptchan)
	}

	okcount = 0

	for i := 0; i < pn.numNodes; i++ {
		ret, ok := <-acceptchan
		if !ok {
			fmt.Println("Didn't finish accept stage even after 15 seconds")
			return errors.New("Didn't finish accept stage even after 15 seconds")
		}
		if ret.Status == paxosrpc.OK {
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
	for _, v := range pn.hostMap {
		fmt.Println("Will call Commit on ", v)
		go commit(pn, v, valueToPropose, args.Key, commitchan)
	}

	for i := 0; i < pn.numNodes; i++ {
		_, ok := <-commitchan
		if !ok {
			fmt.Println("Didn't finish commit stage even after 15 seconds")
			return errors.New("Didn't finish commit stage even after 15 seconds")
		}
	}

	reply.V = valueToPropose

	return nil
}

func (pn *paxosNode) GetValue(args *paxosrpc.GetValueArgs, reply *paxosrpc.GetValueReply) error {
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
}

func (pn *paxosNode) RecvPrepare(args *paxosrpc.PrepareArgs, reply *paxosrpc.PrepareReply) error {
	defer fmt.Println("Leaving RecvPrepare of ", pn.myHostPort)
	fmt.Println("In RecvPrepare of ", pn.myHostPort)
	key := args.Key
	num := args.N

	pn.maxSeqNumSoFarLock.Lock()
	maxNum := pn.maxSeqNumSoFar[key]
	pn.maxSeqNumSoFarLock.Unlock()

	// reject proposal when its proposal number is not higher than the highest number it's ever seen
	if maxNum > num {
		fmt.Println("In RecvPrepare of ", pn.myHostPort, "rejected proposal:", key, num, "maxNum:", maxNum)
		//return -1 for proposal promised
		reply.Status = paxosrpc.Reject
		reply.N_a = -1
		return nil
	}
	// promise proposal when its higher. return with the number and value accepted
	fmt.Println("In RecvPrepare of ", pn.myHostPort, "accepted proposal:", key, num, "maxNum:", maxNum)

	pn.maxSeqNumSoFarLock.Lock()
	pn.maxSeqNumSoFar[key] = num
	pn.maxSeqNumSoFarLock.Unlock()

	// fill reply with accepted seqNum and value. default is -1
	reply.Status = paxosrpc.OK

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

	pn.hostMap[args.SrvID] = args.Hostport
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

// This file contains constants and arguments used to perform RPCs between
// two Paxos nodes. DO NOT MODIFY!

package paxosrpc

// Status represents the status of a RPC's reply.
type Status int
type Lookup int

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
	V   interface{} // Value for the Key
}

type ProposeReply struct {
	V interface{} // Value that was actually committed for that key
}

type GetValueArgs struct {
	Key string
}

type GetValueReply struct {
	V      interface{}
	Status Lookup
}

type PrepareArgs struct {
	Key string
	N   int
}

type PrepareReply struct {
	Status Status
	N_a    int         // Highest proposal number accepted
	V_a    interface{} // Corresponding value
}

type AcceptArgs struct {
	Key string
	N   int
	V   interface{}
}

type AcceptReply struct {
	Status Status
}

type CommitArgs struct {
	Key string
	V   interface{}
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
*/
