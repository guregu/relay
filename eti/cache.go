package eti

import (
	"log"
	"strings"

	"labix.org/v2/mgo"
	//"labix.org/v2/mgo/bson"
	"github.com/guregu/bbs"
)

var dbSesh *mgo.Session
var db *mgo.Database

var dangerTags = map[string]bool{
	"TCF":         true,
	"TCF Lite":    true,
	"Moderations": true,
}

func DBConnect(addr, name string) {
	var err error
	dbSesh, err = mgo.Dial(addr)
	if err != nil {
		log.Fatalf("Couldn't connect to DB (%s): %s\n", addr, err.Error())
	}
	db = dbSesh.DB(name)
	log.Println("connected to db " + addr)
	/*
			lpIndex := mgo.Index{
				Key:        []string{"id"},
				Unique:     true,
				DropDups:   false,
				Background: true,
				Sparse:     true,
			}
			err = db.C("threads").EnsureIndex(lpIndex)

		if err != nil {
			log.Fatalf("Couldn't make index: %s\n", err.Error())
		}
	*/
}

func getThread(id string) *bbs.ThreadMessage {
	if db == nil {
		return nil
	}

	var thread *bbs.ThreadMessage
	err := db.C("threads").FindId(id).One(&thread)
	if err != nil {
		return nil
	}
	return thread
}

func updateThread(thread bbs.ThreadMessage) {
	if db == nil {
		return
	}

	for _, tag := range thread.Tags {
		if dangerTags[tag] || strings.HasSuffix(tag, "(social)") {
			log.Println("Danger!!", tag, thread.ID)
			return
		}
	}

	db.C("threads").UpsertId(thread.ID, thread)
}
