package client

type ClientNode interface {
	// Crawls the internet with given root url until it reaches the number of pages sepecified.
	Crawl(args *CrawlArgs, reply *CrawlReply) error

	// GetLinks fetches all links contained in the given link (if any) from CrawlStore
	GetLinks(args *GetLinksArgs, reply *GetLinksReply) error

	// RunRageRank retrieves all the crawled link relationships from CrawlStore and runs
	// pagerank algorithm on them. It also saves the page rank of each url back to CrawlStore
	// for further references
	RunPageRank(args *PageRankArgs, reply *PageRankReply) error

	// GetRank fetches the page rank for the requested url from CrawlStore
	GetRank(args *GetRankArgs, reply *GetRankReply) error
}
