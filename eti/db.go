package eti

import (
	"log"

	"labix.org/v2/mgo"
	//"labix.org/v2/mgo/bson"
)

var dbSesh *mgo.Session
var db *mgo.Database

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
