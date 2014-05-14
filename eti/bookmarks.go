package eti

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/guregu/bbs"
)

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
		updateBookmarks(username, bookmarks)
	}
	return bookmarks
}
