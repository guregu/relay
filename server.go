package main

import "github.com/tiko-chan/bbs"
import "github.com/tiko-chan/relay/eti"
import "flag"

func main() {
	bind := flag.String("bind", "localhost:8080", "The address to bind on (like localhost:1337).")
	flag.Parse()

	bbs.Serve(*bind, "/bbs", "ETI Relay", "html,tags", "End of the Internet -> BBS Relay", "eti-bbs0.1", factory)
}

func factory() bbs.BBS {
	return new(eti.ETI)
}
