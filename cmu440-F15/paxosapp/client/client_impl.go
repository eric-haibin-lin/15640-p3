package client

import (
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"net/http"
	"github.com/cmu440-F15/paxosapp/common"
	"github.com/cmu440-F15/paxosapp/rpc/clientrpc"
)

type clientNode struct {
	conn common.Conn
	myHostPort string
	
}

// NewClientNode creates a new ClientNode. This function should return only when
// it successfully dials to a master node, and should return a non-nil error if the 
// master node could not be reached.
//
// masterHostPort is the hostname and port number to master node
func NewClientNode(myHostPort string, masterHostPort string) (ClientNode, error) {
	fmt.Println("myhostport is", myHostPort, "hostPort of masterNode to connect is", masterHostPort)

	var a clientrpc.RemoteClientNode

	var conn common.Conn
	conn.HostPort = masterHostPort
	//TODO dial with master hostPort

	node := clientNode{}
	node.conn = conn
	node.myHostPort = myHostPort

	listener, err := net.Listen("tcp", myHostPort)
	if err != nil {
		return nil, err
	}

	a = &node

	err = rpc.RegisterName("ClientNode", clientrpc.Wrap(a))
	if err != nil {
		return nil, err
	}

	rpc.HandleHTTP()
	go http.Serve(listener, nil)

	return a, nil
}

func (cn *clientNode) Crawl(args *clientrpc.CrawlArgs, reply *clientrpc.CrawlReply) error {
	return errors.New("Not implemented yet.")
}

func (cn *clientNode) RunPageRank(args *clientrpc.PageRankArgs, reply *clientrpc.PageRankReply) error {
	return errors.New("Not implemented yet.")
}

func (cn *clientNode) GetTopKPage(args *clientrpc.TopKPageArgs, reply *clientrpc.TopKPageReply) error {
	return errors.New("Not implemented yet.")
}
