package main

import "github.com/BurntSushi/toml"

type config struct {
	Server   servercfg
	Web      webcfg
	Cache    cachecfg
	FourChan sitecfg
	ETI      sitecfg
}

type servercfg struct {
	Host string
}

type webcfg struct {
	Root  string
	Index string
}

type sitecfg struct {
	Path        string
	Name        string
	Description string
	Enabled     bool
	Cache       bool
}

type cachecfg struct {
	Addr string
}

func parseConfig(file string) (cfg config, err error) {
	_, err = toml.DecodeFile(file, &cfg)
	return cfg, err
}
