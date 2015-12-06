package client

import (
	"errors"
	"fmt"
	//	"net"
	//	"net/rpc"
	"crypto/tls"
	"github.com/cmu440-F15/paxosapp/collectlinks"
	"github.com/cmu440-F15/paxosapp/common"
	"net/http"
	"net/url"
)

type Status int

const (
	OK   Status = iota + 1 // Paxos replied OK
	Fail                   // Paxos rejected the message
)

type clientNode struct {
	conn             common.Conn
	myHostPort       string
	linkRelationChan chan linkRelation
	allLinksChan     chan string
	nextLinkChan     chan string
	visited          map[string]bool
	httpClient       http.Client
}

type linkRelation struct {
	follower string
	followee []string
}

type CrawlArgs struct {
	RootUrl string
}

type CrawlReply struct {
	Status Status
}

type TopKPageArgs struct {
	K int // Top K
}

type TopKPageReply struct {
	List []string // The list of top K urls
}

type PageRankArgs struct {
	Iteration int
}

type PageRankReply struct {
	Status Status
}

// NewClientNode creates a new ClientNode. This function should return only when
// it successfully dials to a master node, and should return a non-nil error if the
// master node could not be reached.
//
// masterHostPort is the hostname and port number to master node
func NewClientNode(myHostPort string, masterHostPort string) (ClientNode, error) {
	fmt.Println("myhostport is", myHostPort, "hostPort of masterNode to connect is", masterHostPort)

	var a ClientNode
	var conn common.Conn
	conn.HostPort = masterHostPort
	//TODO dial with master hostPort

	node := clientNode{}
	node.conn = conn
	node.myHostPort = myHostPort

	node.linkRelationChan = make(chan linkRelation)
	node.allLinksChan = make(chan string)
	node.nextLinkChan = make(chan string)
	node.visited = make(map[string]bool)

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
	fmt.Println("Crawl invoked on ", cn.myHostPort)
	//set root url as the starting point of crawling
	rootUrl := args.RootUrl
	go func() { cn.allLinksChan <- rootUrl }()
	go cn.checkVisited()

	//start to crawl!
	for {
		select {
		case uri, ok := <-cn.nextLinkChan:
			if ok {
				_, err := cn.doCrawl(uri)
				if err == nil {
					//TODO call Append on master node to persist the urls crawled
				}
			} else {
				//Should stop crawling now
			}
		}
	}
	return errors.New("Not append to master yet.")
}

func (cn *clientNode) RunPageRank(args *PageRankArgs, reply *PageRankReply) error {
	fmt.Println("RunPageRank invoked on ", cn.myHostPort)
	return errors.New("Not implemented yet.")
}

func (cn *clientNode) GetTopKPage(args *TopKPageArgs, reply *TopKPageReply) error {
	fmt.Println("GetTopKPage invoked on ", cn.myHostPort)
	return errors.New("Not implemented yet.")
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
