// This file contains constants and arguments used to perform RPCs between
// two Client nodes and Master nodes. 

package clientrpc

// Status represents the status of a RPC's reply.
type Status int

const (
	OK     Status = iota + 1 // Paxos replied OK
	Fail                   	// Paxos rejected the message
)


type CrawlArgs struct {
	RootUrl string
}

type CrawlReply struct {
	Status Status 
}

type TopKPageArgs struct {
	K   int // Top K 
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