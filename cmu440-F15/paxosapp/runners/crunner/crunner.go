//The runner for ClientNode

package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"github.com/cmu440-F15/paxosapp/client"
)

var (
	port       = flag.String("port", "", "port for client node")
	opt       = flag.String("opt", "", "option for client node, either crawl or pagerank")
	num       = flag.Int("num", 10, "number of webpages to crawl")
	url       = flag.String("url", "http://news.google.com", "root url to start crawling with")
	masterPort = flag.String("masterPort", "", "port for master node")
)

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
}

func main() {
	flag.Parse()
	
	myHostPort := "localhost:" + *port
	masterHostPort := strings.Split(*masterPort, ",")
	fmt.Println("crunner creates client node on", myHostPort, "with master on ", *masterPort)
	
	// Create and start the Paxos Node.
	cli, err := client.NewClientNode(myHostPort, masterHostPort)
	if err != nil {
		log.Fatalln("Failed to create client node:", err)
	}

	var absoluteUrl string
	if !strings.Contains(*url, "http://")  && !strings.Contains(*url, "https://") {
		absoluteUrl = "http://" + *url 
	}
	
	if *opt == "crawl" {
		fmt.Println("crunner invokes Crawl on client" )
		var args client.CrawlArgs
		var reply client.CrawlReply
		args.RootUrl = absoluteUrl
		args.NumPages = *num
		cli.Crawl(&args, &reply) 	
		
	} else if *opt == "getlink" {
		fmt.Println("crunner invokes GetLink on client" )
		var getLinkArgs client.GetLinkArgs
		var getLinkReply client.GetLinkReply
		cli.GetLink(&getLinkArgs, &getLinkReply)
		
	} else if *opt == "pagerank" {
		fmt.Println("crunner invokes RunPageRank on client" )
		var pageRankArgs client.PageRankArgs
		var pageRankReply client.PageRankReply
		cli.RunPageRank(&pageRankArgs, &pageRankReply)
		
	} else if *opt == "getrank" {
		fmt.Println("crunner invokes GetRank on client" )
		fmt.Println("crunner invokes GetLink on client" )
		var getRankArgs client.GetRankArgs
		var getRankReply client.GetRankReply
		cli.GetRank(&getRankArgs, &getRankReply)
		
	}
}