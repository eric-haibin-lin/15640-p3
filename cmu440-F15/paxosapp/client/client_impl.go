package client

import (
	"errors"
	"fmt"
//	"net"
//	"net/rpc"
	"net/http"
	"github.com/cmu440-F15/paxosapp/common"
	"github.com/cmu440-F15/paxosapp/collectlinks"
	"github.com/cmu440-F15/paxosapp/rpc/clientrpc"
	"net/url"
	"crypto/tls"
	
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

	var a ClientNode

	var conn common.Conn
	conn.HostPort = masterHostPort
	//TODO dial with master hostPort

	node := clientNode{}
	node.conn = conn
	node.myHostPort = myHostPort

	a = &node

	return a, nil
}

// Crawls the entire internet with given root url. 
func (cn *clientNode) Crawl(args *clientrpc.CrawlArgs, reply *clientrpc.CrawlReply) error {
	
	rootUrl := args.RootUrl
	
	queue := make(chan string)
	filteredQueue := make(chan string)

	go func() { queue <- rootUrl }()
	go filterQueue(queue, filteredQueue)

	// pull from the filtered queue, add to the unfiltered queue
	for uri := range filteredQueue {
		enqueue(uri, queue)
	}
	
	//TODO call Append on master node to persist the urls crawled
	
	return errors.New("Not append to master yet.")
}

func (cn *clientNode) RunPageRank(args *clientrpc.PageRankArgs, reply *clientrpc.PageRankReply) error {
	return errors.New("Not implemented yet.")
}

func (cn *clientNode) GetTopKPage(args *clientrpc.TopKPageArgs, reply *clientrpc.TopKPageReply) error {
	return errors.New("Not implemented yet.")
}

// Internal method for crawler
func filterQueue(in chan string, out chan string) {
	var seen = make(map[string]bool)
	for val := range in {
		if !seen[val] {
			seen[val] = true
			out <- val
		}
	}
}

func enqueue(uri string, queue chan string) {
	fmt.Println("fetching", uri)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := http.Client{Transport: transport}
	resp, err := client.Get(uri)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()

	links := collectlinks.All(resp.Body)

	for _, link := range links {
		absolute := fixUrl(link, uri)
		if uri != "" {
			go func() { queue <- absolute }()
		}
	}

}

func fixUrl(href, base string) string {
	uri, err := url.Parse(href)
	if err != nil {
		return ""
	}
	baseUrl, err := url.Parse(base)
	if err != nil {
		return ""
	}
	uri = baseUrl.ResolveReference(uri)
	return uri.String()
}
