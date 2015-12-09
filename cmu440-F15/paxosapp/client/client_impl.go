package client

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/cmu440-F15/paxosapp/collectlinks"
	"github.com/cmu440-F15/paxosapp/common"
	"github.com/cmu440-F15/paxosapp/pagerank"
	"github.com/cmu440-F15/paxosapp/rpc/paxosrpc"
	"math/rand"
	"net/http"
	"net/rpc"
	"time"
)

type Status int

const (
	OK   Status = iota + 1 // Paxos replied OK
	Fail                   // Paxos rejected the message
)

const (
	tolerance         = 0.0001 //the default tolerance when running pagerank algorithm
	followProbability = 0.85   //the defaul probabilitic assumption of following a link
)

type clientNode struct {
	conn       common.Conn
	myHostPort string
	//a channel to receive all url's relationship before writing to CrawlStore
	linkRelationChan chan linkRelation
	allLinksChan     chan string
	//a channel to receive all url's to crawl next time
	nextLinkChan chan string
	//a map to indicate whether the link has been crawled or not
	visited map[string]bool
	//the http client who fetches the page
	httpClient http.Client
	//a mapping between the url and its id for pagerank calculation
	idMap  map[string]int
	urlMap map[int]string
	nextId int
}

type linkRelation struct {
	follower string
	followee []string
}

type CrawlArgs struct {
	RootUrl  string
	NumPages int
}

type CrawlReply struct {
	Status Status
}

type GetLinksArgs struct {
	Url string
}

type GetLinksReply struct {
	List []string // The list of all urls
}

type GetRankArgs struct {
	Url string
}

type GetRankReply struct {
	Value float64
}

type PageRankArgs struct {
	//Nothing to provide here
}

type PageRankReply struct {
	Status Status
}

// NewClientNode creates a new ClientNode. This function should return only when
// it successfully dials to a master node, and should return a non-nil error if the
// master node could not be reached.
//
// masterHostPort is the list of hostnames and port numbers to master nodes
func NewClientNode(myHostPort string, masterHostPort []string) (ClientNode, error) {
	defer fmt.Println("Leaving NewClientNode")
	fmt.Println("myhostport is", myHostPort, ", hostPort of masterNode to connect is", masterHostPort)

	var a ClientNode
	//Pick a master node and dial on it
	index := rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(masterHostPort))
	numRetries := 3
	dialer, err := rpc.DialHTTP("tcp", masterHostPort[index])
	for err != nil {
		numRetries -= 1
		dialer, err = rpc.DialHTTP("tcp", masterHostPort[index])
		if numRetries <= 0 {
			return nil, errors.New("Fail to dial to master:" + masterHostPort[index])
		}
	}
	//Save the hostport and dialer for master node, initialize basic member variables
	var conn common.Conn
	conn.HostPort = masterHostPort
	conn.Dialer = dialer
	node := clientNode{}
	node.conn = conn
	node.myHostPort = myHostPort
	node.linkRelationChan = make(chan linkRelation)
	node.allLinksChan = make(chan string)
	node.nextLinkChan = make(chan string)
	node.visited = make(map[string]bool)
	node.idMap = make(map[string]int)
	node.urlMap = make(map[int]string)

	// Initialize the http client for the crawler
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	node.httpClient = http.Client{Transport: transport}

	a = &node
	return a, nil
}

// Crawls the internet with given root url until it reaches the number of pages sepecified.
func (cn *clientNode) Crawl(args *CrawlArgs, reply *CrawlReply) error {
	defer fmt.Println("Leavin Crawl on client", cn.myHostPort)
	fmt.Println("Crawl invoked on ", cn.myHostPort)

	//set root url as the starting point of crawling
	rootUrl := args.RootUrl
	go func() { cn.allLinksChan <- rootUrl }()
	go cn.checkVisited()

	//start to crawl and count the number of pages crawled!
	count := 0
	for {
		select {
		case uri := <-cn.nextLinkChan:
			//get the next link and crawl all pages starting from there
			if count < args.NumPages {
				rel, err := cn.doCrawl(uri)
				if err == nil {
					count += 1
					//Writing the crawl result to CrawlStore
					var appendArgs paxosrpc.AppendArgs
					appendArgs.Key = rel.follower
					appendArgs.Value = rel.followee
					var appendReply paxosrpc.AppendReply
					fmt.Println("Calling PaxosNode.Append")
					for _, v := range appendArgs.Value {
						fmt.Println("Writing", v, "with key", appendArgs.Key)
					}
					cn.conn.Dialer.Call("PaxosNode.Append", &appendArgs, &appendReply)
				}
			} else {
				return nil
			}
		}
	}
	return nil
}

// RunRageRank retrieves all the crawled link relationships from CrawlStore and runs
// pagerank algorithm on them. It also saves the page rank of each url back to CrawlStore
// for further references
func (cn *clientNode) RunPageRank(args *PageRankArgs, reply *PageRankReply) error {
	fmt.Println("RunPageRank invoked on ", cn.myHostPort)

	//Get all page relationships from CrawlStore
	var getAllLinksArgs paxosrpc.GetAllLinksArgs
	var getAllLinksReply paxosrpc.GetAllLinksReply
	fmt.Println("Invoking PaxosNode.GetAllLinks on", cn.conn.HostPort)
	err := cn.conn.Dialer.Call("PaxosNode.GetAllLinks", &getAllLinksArgs, &getAllLinksReply)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("Calculating page rank on all links...")
	//calculate page rank!
	pageRankEngine := pagerank.New()
	for k, list := range getAllLinksReply.LinksMap {
		followerId := cn.getId(k)
		for _, v := range list {
			followeeId := cn.getId(v)
			pageRankEngine.Link(followerId, followeeId)
		}
	}
	//Save all page rank results to CrawlStore
	pageRankEngine.Rank(followProbability, tolerance, func(label int, rank float64) {
		fmt.Println(cn.urlMap[label], rank*100)
		var putRankArgs paxosrpc.PutRankArgs
		putRankArgs.Key = cn.urlMap[label]
		putRankArgs.Value = rank * 100
		var putRankReply paxosrpc.PutRankReply
		err := cn.conn.Dialer.Call("PaxosNode.PutRank", &putRankArgs, &putRankReply)
		if err != nil {
			fmt.Println(err)
		}
	})
	return nil
}

// getId returns the id mapped from given url
func (cn *clientNode) getId(url string) int {
	id, ok := cn.idMap[url]
	//If url doesn't exist in the current url - id map, create a new mapping for it
	if !ok {
		cn.idMap[url] = cn.nextId
		cn.urlMap[cn.nextId] = url
		cn.nextId += 1
	}
	return id
}

// GetRank fetches the page rank for the requested url from CrawlStore
func (cn *clientNode) GetRank(args *GetRankArgs, reply *GetRankReply) error {
	fmt.Println("GetRank invoked on ", cn.myHostPort)
	var rankArgs paxosrpc.GetRankArgs
	rankArgs.Key = args.Url
	var rankReply paxosrpc.GetRankReply
	err := cn.conn.Dialer.Call("PaxosNode.GetRank", &rankArgs, &rankReply)
	if err != nil {
		fmt.Println(err)
		return err
	}
	fmt.Println("GetRank returns value:", rankReply.Value)
	reply.Value = rankReply.Value
	return nil
}

// GetLinks fetches all links contained in the given link (if any)
func (cn *clientNode) GetLinks(args *GetLinksArgs, reply *GetLinksReply) error {
	fmt.Println("GetLink invoked on ", cn.myHostPort)
	var linksArgs paxosrpc.GetLinksArgs
	linksArgs.Key = args.Url
	var linksReply paxosrpc.GetLinksReply
	fmt.Println("Calling PaxosNode.GetLinks with key", linksArgs.Key)
	err := cn.conn.Dialer.Call("PaxosNode.GetLinks", &linksArgs, &linksReply)
	if err != nil {
		fmt.Println(err)
		return err
	}
	fmt.Println("GetLinks returns list:")
	for _, v := range linksReply.Value {
		fmt.Println(v)
	}
	reply.List = linksReply.Value
	return nil
}

// filterVisited checks if all links crawled are visited. If not, push it to the
// nextLinksChan to start crawling from there
func (cn *clientNode) checkVisited() {
	for val := range cn.allLinksChan {
		if !cn.visited[val] {
			cn.visited[val] = true
			cn.nextLinkChan <- val
		}
	}
}

// doCrawl fetches the webpage and collects the links contained in this webpage
func (cn *clientNode) doCrawl(uri string) (linkRelation, error) {
	var rel linkRelation
	rel.follower = common.RemoveHttp(uri)
	rel.followee = make([]string, 0)
	resp, err := cn.httpClient.Get(uri)
	if err != nil {
		fmt.Println(err)
		return rel, errors.New("Failed to fetch page" + uri)
	}
	defer resp.Body.Close()
	fmt.Println("fetched ", uri)
	links := collectlinks.All(resp.Body)
	for _, link := range links {
		absolute := common.GetAbsoluteUrl(link, uri)
		if absolute != "" {
			go func() { cn.allLinksChan <- absolute }()
			removeHttps := common.RemoveHttp(absolute)
			rel.followee = append(rel.followee, removeHttps)
		}
	}
	/*fmt.Print("New link relation: ", rel.follower, " ========> ")
	for _, followee := range rel.followee {
		fmt.Print(followee, "")
	}
	fmt.Println()*/
	return rel, nil
}
