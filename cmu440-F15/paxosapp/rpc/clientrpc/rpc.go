package clientrpc



type RemoteClientNode interface {
	//Crawl takes the root url where crawling starts and crawls for a certain time period.
	//The crawling ends either when it hits the time limit or when it successfully reaches
	//the target number of pages. It should reply with either OK or Fail status.
	Crawl(args *CrawlArgs, reply *CrawlReply) error
	
	//RunPageRank runs the page rank algorithm based on all crawled urls stored in backend.
	//It either runs the algorithm with specified iteration, or reaches a threshold where 
	//result is good enough. It finally stores the result in backend.
	RunPageRank(args *PageRankArgs, reply *PageRankReply) error
	
	
	GetTopKPage(args *TopKPageArgs, reply *TopKPageReply) error
}

type ClientNode struct {
	// Embed all methods into the struct. See the Effective Go section about
	// embedding for more details: golang.org/doc/effective_go.html#embedding
	RemoteClientNode
}

// Wrap wraps t in a type-safe wrapper struct to ensure that only the desired
// methods are exported to receive RPCs.
func Wrap(t RemoteClientNode) RemoteClientNode {
	return &ClientNode{t}
}
