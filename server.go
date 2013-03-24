package main

import "github.com/tiko-chan/bbs"
import "github.com/tiko-chan/relay/eti"
import "flag"

func main() {
	bind := flag.String("bind", "localhost:8080", "The address to bind on (like localhost:1337).")
	flag.Parse()
	hello := bbs.HelloMessage{
		Command:         "hello",
		Name:            "ETI Relay",
		ProtocolVersion: 0,
		Description:     "End of the Internet -> BBS Relay",
		Options:         []string{"tags"},
		Access: bbs.AccessInfo{
			// There are no guest commands.
			UserCommands: []string{"get", "list", "post", "reply", "info"},
		},
		Formats:       []string{"html", "text"},
		ServerVersion: "eti-relay 0.1",
	}

	bbs.Serve(*bind, "/bbs", hello, factory)
}

func factory() bbs.BBS {
	return new(eti.ETI)
}
