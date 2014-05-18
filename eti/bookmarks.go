package eti

import (
	"log"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/guregu/bbs"
	"github.com/pmylund/go-cache"
)

// map[username][]bbs.Bookmark
var bookmarkCache = cache.New(24*time.Hour, 1*time.Hour)

// findBookmarks returns a user's bookmarks given the root document
func findBookmarks(username string, doc *goquery.Document) []bbs.Bookmark {
	var bookmarks []bbs.Bookmark
	doc.Find("#bookmarks span").Each(func(i int, s *goquery.Selection) {
		a := s.Find("a").First()
		href, _ := a.Attr("href")
		if href != "#" {
			split := strings.Split(href, "/")
			query := split[len(split)-1]
			name := a.Text()
			if name != "[edit]" {
				bookmarks = append(bookmarks, bbs.Bookmark{
					Name:  name,
					Query: query,
				})
			}
		}
	})
	if bookmarks != nil {
		go updateBookmarks(username, bookmarks)
	}
	return bookmarks
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
	bookmarkCache.Set(username, bookmarks, 0)
}
