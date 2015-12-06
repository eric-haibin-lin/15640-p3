//The runner for ClientNode

package main

import (
	"flag"
	"fmt"
	"log"
	"github.com/cmu440-F15/paxosapp/client"
)

var (
	port       = flag.String("port", "", "port for client node")
	masterPort = flag.String("masterPort", "", "port for master node")
)

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
}

func main() {
	flag.Parse()

	fmt.Println("crunner creates client node on localhost:", *port, "with master on localhost:", *masterPort)
	myHostPort := "localhost:" + *port
	masterHostPort := "localhost:" + *masterPort
	
	// Create and start the Paxos Node.
	cli, err := client.NewClientNode(myHostPort, masterHostPort)
	if err != nil {
		log.Fatalln("Failed to create client node:", err)
	}

	var args client.CrawlArgs
	var reply client.CrawlReply
	args.RootUrl = "http://news.google.com"
	cli.Crawl(&args, &reply)
	
	// Run the client node forever.
	select {}
}