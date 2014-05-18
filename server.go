package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"github.com/guregu/bbs"
	"github.com/guregu/relay/eti"
	"github.com/guregu/relay/fourchan"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
)

var cfgFile = flag.String("config", "config.toml", "config file path")
var cfg config
var servers []relay

type relay struct {
	server *bbs.Server
	Path   string `json:"path"`
}

func main() {
	flag.Parse()
	var err error
	cfg, err = parseConfig(*cfgFile)
	if err != nil {
		log.Fatal(err)
	}

	if cfg.Server.Host == "" {
		log.Fatalf("No host set in %s", *cfgFile)
	}

	if cfg.ETI.Enabled {
		path := maybe(cfg.ETI.Path, "/bbs")
		wsPath := ws(path)
		eti.Setup(cfg.ETI.Name, cfg.ETI.Description, wsPath)
		if cfg.ETI.Cache && cfg.Cache.Addr != "" {
			eti.DBConnect(cfg.Cache.Addr, "eti")
		}
		srv := bbs.NewServer(eti.New)
		goji.Handle(path, srv)
		goji.Handle(path+"/ws", srv.WS)
		servers = append(servers, relay{
			server: srv,
			Path:   path,
		})
	}

	if cfg.FourChan.Enabled {
		path := maybe(cfg.FourChan.Path, "/bbs")
		wsPath := ws(path)
		fourchan.Setup(cfg.FourChan.Name, cfg.FourChan.Description, wsPath)
		// no cache yet. sorry moot
		srv := bbs.NewServer(fourchan.New)
		goji.Handle(path, srv)
		goji.Handle(path+"/ws", srv.WS)
		servers = append(servers, relay{
			server: srv,
			Path:   path,
		})
	}

	if cfg.Web.Index != "" {
		log.Println(cfg.Web.Index)
		goji.Get(cfg.Web.Index, indexHandler)
	}

	if cfg.Web.Root != "" {
		goji.Get("/*", http.FileServer(http.Dir(cfg.Web.Root)))
	}

	goji.Serve()
}

// for index.json, which lists all our servers
func indexHandler(c web.C, w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	data, err := json.Marshal(servers)
	log.Printf("%s", string(data))
	if err != nil {
		log.Println(err)
	}
	w.Write(data)
}

func maybe(test, def string) string {
	if test == "" {
		return def
	}
	return test
}

func ws(path string) string {
	return "ws://" + cfg.Server.Host + path + "/ws"
}
