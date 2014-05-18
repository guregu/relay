package eti

import (
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/go.net/html"
	"code.google.com/p/go.net/html/atom"
	"github.com/PuerkitoBio/goquery"
	"github.com/guregu/bbs"
	_ "labix.org/v2/mgo/bson"
)

var (
	accessDeniedError = errors.New("access denied")
	serverIsDownError = errors.New("remote server")
)

var dangerTags = map[string]bool{
	"TCF":         true,
	"TCF Lite":    true,
	"Moderations": true,
}

type metadata struct {
	ID       string `bson:"_id"`
	Thread   bbs.ThreadMessage
	Archived bool
	Updated  time.Time
	Access   map[string]bool `bson:",omitempty"`

	pages int
}

func getThread(id string) *metadata {
	if db == nil {
		return nil
	}

	var md *metadata
	err := db.C("threads").FindId(id).One(&md)
	if err != nil {
		return nil
	}
	return md
}

func updateThread(md metadata) {
	if db == nil {
		return
	}

	for _, tag := range md.Thread.Tags {
		if dangerTags[tag] || strings.HasSuffix(tag, "(social)") {
			log.Println("Danger!!", tag, md.Thread.ID)
			return
		}
	}

	db.C("threads").UpsertId(md.ID, md)
}

func parseToken(token string) (bbs.Range, bool) {
	last, err := strconv.Atoi(token)
	if err != nil {
		return bbs.Range{}, false
	} else {
		return bbs.Range{last + 1, last + DefaultRange.End}, true
	}
}

func pages(r bbs.Range) (startPage, endPage int) {
	startPage = int(math.Floor(float64(r.Start)/ETITopicsPerPage) + 1)
	endPage = int(math.Floor(float64(r.End)/ETITopicsPerPage) + 1)
	return
}

func threadURLs(id string, fetchRange bbs.Range, archived bool) ([]string, error) {
	startPage, endPage := pages(fetchRange)
	// topics can only have 500 pages, 501 sometimes
	// 555 pages as a max is a 'safe bet'!
	if startPage > endPage || endPage-startPage > 555 {
		return nil, errors.New("invalid range")
	}
	subdomain := "boards"
	if archived {
		subdomain = "archives"
	}

	var urls []string
	for i := int(startPage); i <= int(endPage); i++ {
		urls = append(urls, fmt.Sprintf("http://%s.endoftheinter.net/showmessages.php?topic=%s&page=%d&u=%s", subdomain, id, i, ""))
	}
	return urls, nil
}

// takes a <script> selection and turns ETI lazy loaded images into <img> tags
func transmuteImages(i int, sel *goquery.Selection) {
	a := sel.Parent()
	url, ok := a.Attr("imgsrc")
	if ok {
		for a_i := range a.Nodes[0].Attr {
			if a.Nodes[0].Attr[a_i].Key == "href" {
				a.Nodes[0].Attr[a_i].Val = url
			}
		}
		// can't believe this works
		script := sel.Nodes[0]
		script.Type = html.ElementNode
		script.Data = "img"
		script.DataAtom = atom.Image
		script.FirstChild = nil
		script.Attr = []html.Attribute{html.Attribute{"", "src", url}}
	}
}
