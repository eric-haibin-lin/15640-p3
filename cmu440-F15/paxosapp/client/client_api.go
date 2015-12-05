package client

import (
	"github.com/cmu440-F15/paxosapp/rpc/clientrpc"
)
// Crawl crawls 100 urls from the given root url and stores that in the data store
// RunPageRank runs the page rank algorithm on all urls collected
// GetTopKPage returns the top k websites ordered by pagerank algorithm
type ClientNode interface {
	Crawl(args *clientrpc.CrawlArgs, reply *clientrpc.CrawlReply) error
	RunPageRank(args *clientrpc.PageRankArgs, reply *clientrpc.PageRankReply) error
	GetTopKPage(args *clientrpc.TopKPageArgs, reply *clientrpc.TopKPageReply) error
}