package main

import "github.com/tiko-chan/bbs"
import "github.com/tiko-chan/relay/fourchan"
import "flag"

//import "github.com/tiko-chan/relay/eti"

func main() {
	bind := flag.String("bind", "localhost:8080", "The address to bind on (like localhost:1337).")
	flag.Parse()

	//bbs.Serve(*bind, "/bbs", eti.Hello, func() bbs.BBS { return new(eti.ETI) })
	bbs.Serve(*bind, "/bbs", fourchan.Hello, func() bbs.BBS { return new(fourchan.Fourchan) })
}
