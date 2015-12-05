package client

import (
	"errors"
	"github.com/cmu440-F15/paxosapp/rpc/clientrpc"
)

type clientNode struct {
	
}

func NewClientNode(myHostPort string, masterHost string) (ClientNode, error) {
	return nil, errors.New("Not implemented yet.")
}

func (cn *clientNode) Crawl(args *clientrpc.CrawlArgs, reply *clientrpc.CrawlReply) error{
	return errors.New("Not implemented yet.")
}

func (cn *clientNode) RunPageRank(args *clientrpc.PageRankArgs, reply *clientrpc.PageRankReply) error {
	return errors.New("Not implemented yet.")
}

func (cn *clientNode) GetTopKPage(args *clientrpc.TopKPageArgs, reply *clientrpc.TopKPageReply) error{
	return errors.New("Not implemented yet.")	
}