package eti

import (
	"log"
	"strings"
	"time"

	"labix.org/v2/mgo"
	//"labix.org/v2/mgo/bson"
	"github.com/guregu/bbs"
	"github.com/pmylund/go-cache"
)

var dbSesh *mgo.Session
var db *mgo.Database

// map[username][]bookmark
var bookmarkCache = cache.New(24*time.Hour, 1*time.Hour)

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

func getBookmarks(username string) []bbs.Bookmark {
	bookmarks, ok := bookmarkCache.Get(username)
	if !ok {
		log.Println("no cache bookmark " + username)
		return nil
	}
	log.Println("cache bookmark :) " + username)
	return bookmarks.([]bbs.Bookmark)
}

func updateBookmarks(username string, bookmarks []bbs.Bookmark) {
	log.Printf("upd8 bookmark %#v", bookmarks)
	bookmarkCache.Set(username, bookmarks, 0)
}
