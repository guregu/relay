package main

import "github.com/guregu/bbs"
import "github.com/guregu/relay/fourchan"
import "github.com/guregu/relay/eti"
import "github.com/laurent22/toml-go/toml"

func main() {
	//bbs.Serve(*bind, "/bbs", eti.Hello, func() bbs.BBS { return new(eti.ETI) })
	//bbs.Serve(*bind, "/bbs", fourchan.Hello, func() bbs.BBS { return new(fourchan.Fourchan) })

	servers := 0

	var parser toml.Parser
	conf := parser.ParseFile("config.toml")

	etiEnabled := conf.GetBool("eti.enabled", false)
	fourchanEnabled := conf.GetBool("fourchan.enabled", false)

	if etiEnabled {
		eti.Hello.Name = conf.GetString("eti.name", eti.Hello.Name)
		eti.Hello.Description = conf.GetString("eti.description", eti.Hello.Description)
		bbs.Serve(conf.GetString("eti.bind", "localhost:8080"), conf.GetString("eti.path", "/bbs"), func() bbs.BBS { return new(eti.ETI) })
		servers++
	}

	if fourchanEnabled {
		fourchan.Hello.Name = conf.GetString("fourchan.name", fourchan.Hello.Name)
		fourchan.Hello.Description = conf.GetString("fourchan.description", fourchan.Hello.Description)
		bbs.Serve(conf.GetString("fourchan.bind", "localhost:8080"), conf.GetString("fourchan.path", "/bbs"), func() bbs.BBS { return new(fourchan.Fourchan) })
		servers++
	}
}
