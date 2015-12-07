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
	"net/url"
	"time"
)

type Status int

const (
	OK   Status = iota + 1 // Paxos replied OK
	Fail                   // Paxos rejected the message
)

const (
	tolerance = 0.0001
)

type clientNode struct {
	conn             common.Conn
	myHostPort       string
	linkRelationChan chan linkRelation
	allLinksChan     chan string
	nextLinkChan     chan string
	visited          map[string]bool
	httpClient       http.Client
	idMap            map[string]int
	urlMap           map[int]string
	nextId           int
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
	List []string // The list of top K urls
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

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	node.httpClient = http.Client{Transport: transport}

	a = &node

	return a, nil
}

// Crawls the entire internet with given root url.
func (cn *clientNode) Crawl(args *CrawlArgs, reply *CrawlReply) error {
	defer fmt.Println("Leavin Crawl on client", cn.myHostPort)
	fmt.Println("Crawl invoked on ", cn.myHostPort)

	//set root url as the starting point of crawling
	rootUrl := args.RootUrl
	go func() { cn.allLinksChan <- rootUrl }()
	go cn.checkVisited()

	count := 0
	//start to crawl!
	for {
		select {
		case uri := <-cn.nextLinkChan:
			if count < args.NumPages {
				rel, err := cn.doCrawl(uri)
				if err == nil {
					count += 1
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

func (cn *clientNode) RunPageRank(args *PageRankArgs, reply *PageRankReply) error {
	fmt.Println("RunPageRank invoked on ", cn.myHostPort)

	//Get all page relationships from data store
	var getAllLinksArgs paxosrpc.GetAllLinksArgs
	var getAllLinksReply paxosrpc.GetAllLinksReply
	fmt.Println("Invoking PaxosNode.GetAllLinks on", cn.conn.HostPort)
	err := cn.conn.Dialer.Call("PaxosNode.GetAllLinks", &getAllLinksArgs, &getAllLinksReply)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("Calculating page rank...")
	//calculate page rank!
	pageRankEngine := pagerank.New()
	for k, list := range getAllLinksReply.LinksMap {
		followerId := cn.getId(k)
		for _, v := range list {
			followeeId := cn.getId(v)
			pageRankEngine.Link(followerId, followeeId)
		}
	}

	pageRankEngine.Rank(0.85, tolerance, func(label int, rank float64) {
		fmt.Println(cn.urlMap[label], rank*100)
		//TODO call PutRank() on Master Node
		//		rankAsPercentage := toPercentage(rank)
		//		if math.Abs(rankAsPercentage - expected[label]) > tolerance {
		//			t.Error("Rank for", label, "should be", expected[label], "but was", rankAsPercentage)
		//		}
	})
	return nil
}

// getId returns the id mapped from given url
func (cn *clientNode) getId(url string) int {
	id, ok := cn.idMap[url]
	//The url doesn't exist in the current url - id map
	if !ok {
		cn.idMap[url] = cn.nextId
		cn.urlMap[cn.nextId] = url
		cn.nextId += 1
	}
	return id
}

func (cn *clientNode) GetRank(args *GetRankArgs, reply *GetRankReply) error {
	fmt.Println("GetRank invoked on ", cn.myHostPort)

	//	var rankArgs paxosrpc.GetRankArgs
	//	rankArgs.Key = args.Url
	//	var rankReply paxosrpc.GetRankReply
	//	err := cn.conn.Dialer.Call("PaxosNode.GetRank", &rankArgs, &rankReply)
	//	if err != nil {
	//		fmt.Println(err)
	//		return err
	//	}
	//	fmt.Println("GetLink returns list:")
	//	for _, v := range linksReply.Value {
	//		fmt.Println(v)
	//	}
	return nil
}

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
	rel.follower = uri
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
		absolute := getAbsoluteUrl(link, uri)
		if absolute != "" {
			rel.followee = append(rel.followee, absolute)
			go func() { cn.allLinksChan <- absolute }()
		}
	}
	/*fmt.Print("New link relation: ", rel.follower, " ========> ")
	for _, followee := range rel.followee {
		fmt.Print(followee, "")
	}
	fmt.Println()*/
	return rel, nil
}

// getAbsoluteUrl returns the absolute url based on href contained in the webpage
// and the base url information. (e.g. it turns a /language_tool href link in
// http://www.google.com into http://www.google.com/language_tool)
func getAbsoluteUrl(href, base string) string {
	uri, err := url.Parse(href)
	//abandon broken href
	if err != nil {
		return ""
	}
	//abandon broken base url
	baseUrl, err := url.Parse(base)
	if err != nil {
		return ""
	}
	//resolve and generate absolute url
	uri = baseUrl.ResolveReference(uri)
	return uri.String()
}
