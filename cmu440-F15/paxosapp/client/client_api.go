package client

// Crawl crawls 100 urls from the given root url and stores that in the data store
// RunPageRank runs the page rank algorithm on all urls collected
// GetTopKPage returns the top k websites ordered by pagerank algorithm
type ClientNode interface {
	Crawl(args *CrawlArgs, reply *CrawlReply) error
	RunPageRank(args *PageRankArgs, reply *PageRankReply) error
	GetTopKPage(args *TopKPageArgs, reply *TopKPageReply) error
}
