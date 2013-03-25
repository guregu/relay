package fourchan

import "github.com/tiko-chan/bbs"
import "io/ioutil"
import "net/http"
import "fmt"
import "encoding/json"
import "strconv"
import "code.google.com/p/go.net/html"
import "github.com/tiko-chan/goquery"
import "strings"

const threadURL = "http://api.4chan.org/%s/res/%s.json"
const boardURL = "http://api.4chan.org/%s/%d.json"
const catalogURL = "http://api.4chan.org/%s/catalog.json"
const boardListURL = "http://api.4chan.org/boards.js"
const imageURL = "http://images.4chan.org/%s/src/%d%s"
const thumbnailURL = "http://thumbs.4chan.org/%s/thumb/%d%s"
const spoilerImageURL = "http://static.4chan.org/image/spoiler.png"

var Hello = bbs.HelloMessage{
	Command:         "hello",
	Name:            "◯chan relay",
	ProtocolVersion: 0,
	Description:     "4chan -> BBS Relay",
	Options:         []string{"imageboard", "readonly", "boards"},
	Access: bbs.AccessInfo{
		// There are no user commands.
		GuestCommands: []string{"hello", "get", "list"},
	},
	Formats:       []string{"html", "text"},
	ServerVersion: "4chan-relay 0.1",
}

type Fourchan struct {
}

// we will only define the stuff we need

type Catalog struct {
	Pages []Page `json:"pages"`
}

type Page struct {
	Number  int             `json:"page"`
	Threads []*FourchanPost `json:"threads"`
}

type Thread struct {
	Posts []*FourchanPost `json:"posts"`
}

type FourchanPost struct {
	Number        int    `json:"no"`
	ReplyTo       int    `json:"resto"`
	Sticky        int    `json:"sticky,omitempty"`
	Closed        int    `json:"closed,omitempty"`
	Date          string `json:"now"`
	Timestamp     int    `json:"time"`
	Name          string `json:"name,omitempty"` //username
	Tripcode      string `json:"trip,omitempty"`
	ID            string `json:"id,omitempty"` //user ID
	Capcode       string `json:"capcode,omitempty"`
	CountryName   string `json:"country_name,omitempty"`
	Email         string `json:"email,omitempty"`
	Subject       string `json:"sub,omitempty"`
	Text          string `json:"com,omitempty"` //HTML
	FileTime      uint64 `json:"tim,omitempty"`
	FileExt       string `json:"ext,omitempty"`
	FileDeleted   int    `json:"filedeleted,omitempty"`
	Spoiler       int    `json:"spoiler,omitempty"`
	OmitedPosts   int    `json:"omitted_posts,omitempty"`
	OmittedImages int    `json:"omitted_images,omitempty"`
	Replies       int    `json:"replies,omitempty"`
	Images        int    `json:"images,omitempty"`
}

func (f *Fourchan) LogIn(m *bbs.LoginCommand) bool {
	return false
}

func (f *Fourchan) LogOut(m *bbs.LogoutCommand) *bbs.OKMessage {
	return nil
}

func (f *Fourchan) IsLoggedIn() bool {
	return false
}

func (f *Fourchan) Get(m *bbs.GetCommand) (tm *bbs.ThreadMessage, em *bbs.ErrorMessage) {
	url := fmt.Sprintf(threadURL, m.Board, m.ThreadID)
	data, code := getBytes(url, false)
	if code == 404 {
		return nil, &bbs.ErrorMessage{"error", "get", fmt.Sprintf("Thread /%s/%s not found.", m.Board, m.ThreadID)}
	} else if code != 200 {
		return nil, &bbs.ErrorMessage{"error", "get", fmt.Sprintf("4chan error %d", code)}
	}

	//4chan json in
	var c = Thread{}
	json.Unmarshal(data, &c)

	if len(c.Posts) == 0 {
		return nil, &bbs.ErrorMessage{"error", "get", "No posts!"}
	}

	//bbs json out
	var messages []*bbs.Message
	op := c.Posts[0]

	for i := range c.Posts {
		t := c.Posts[i]
		text := t.Text
		if m.Format == "text" {
			text = unhtml(text)
		}

		thumb := fmt.Sprintf(thumbnailURL, m.Board, t.FileTime, t.FileExt)
		if t.Spoiler != 0 {
			thumb = spoilerImageURL
		}

		if t.FileTime != 0 {
			messages = append(messages, &bbs.Message{
				ID:           strconv.Itoa(t.Number),
				Author:       name(t),
				AuthorID:     t.ID,
				Date:         t.Date,
				Text:         text,
				PictureURL:   fmt.Sprintf(imageURL, m.Board, t.FileTime, t.FileExt),
				ThumbnailURL: thumb,
			})
		} else {
			messages = append(messages, &bbs.Message{
				ID:       strconv.Itoa(t.Number),
				Author:   name(t),
				AuthorID: t.ID,
				Date:     t.Date,
				Text:     text,
			})
		}
	}

	return &bbs.ThreadMessage{
		Command:  "msg",
		ID:       strconv.Itoa(op.Number),
		Title:    op.Subject,
		Board:    m.Board,
		Format:   "html",
		Messages: messages,
	}, nil
}

func (f *Fourchan) List(m *bbs.ListCommand) (lm *bbs.ListMessage, em *bbs.ErrorMessage) {
	data, code := getBytes(fmt.Sprintf(catalogURL, m.Query), true)
	if code == 404 {
		return nil, &bbs.ErrorMessage{"error", "list", fmt.Sprintf("Board /%s/ not found.", m.Query)}
	} else if code != 200 {
		return nil, &bbs.ErrorMessage{"error", "list", fmt.Sprintf("4chan error %d", code)}
	}

	var c = Catalog{}
	json.Unmarshal(data, &c)

	var threads []*bbs.ThreadListing

	//turn this into bbs messages
	for page := range c.Pages {
		for i := range c.Pages[page].Threads {
			t := c.Pages[page].Threads[i]

			title := t.Subject
			if title == "" {
				title = summary(html.UnescapeString(t.Text))
			}

			thumb := fmt.Sprintf(thumbnailURL, m.Query, t.FileTime, t.FileExt)
			if t.Spoiler != 0 {
				thumb = spoilerImageURL
			}

			threads = append(threads, &bbs.ThreadListing{
				ID:           strconv.Itoa(t.Number),
				Title:        title,
				Author:       name(t),
				AuthorID:     t.ID,
				Date:         t.Date,
				PostCount:    t.Replies,
				PictureURL:   fmt.Sprintf(imageURL, m.Query, t.FileTime, t.FileExt),
				ThumbnailURL: thumb,
			})
		}
	}

	lm = &bbs.ListMessage{
		Command: "list",
		Type:    "thread",
		Query:   m.Query,
		Threads: threads,
	}
	return lm, nil
}

func (f *Fourchan) Reply(m *bbs.ReplyCommand) (ok *bbs.OKMessage, em *bbs.ErrorMessage) {
	return nil, &bbs.ErrorMessage{"error", "reply", "This relay is read-only."}
}

func (f *Fourchan) Post(m *bbs.PostCommand) (pm *bbs.OKMessage, em *bbs.ErrorMessage) {
	return nil, &bbs.ErrorMessage{"error", "post", "This relay is read-only."}
}

// takes the first line of a thread's comment for when it has no title
func summary(msg string) string {
	msg = unhtml(msg)
	lines := strings.Split(msg, "\n")
	if len(lines) > 0 {
		msg = lines[0]
	} else {
		return ""
	}

	l := 80
	if len(msg) <= l {
		return msg[0:len(msg)]
	}

	return msg[0:l] + "..."
}

func br2nl(h string) string {
	return strings.Replace(h, "<br>", "\n", -1)
}

func unhtml(s string) string {
	return stringToDocument(br2nl(s)).Text()
}

func name(t *FourchanPost) string {
	var username string
	if t.Name == "" && t.Tripcode == "" && t.Capcode == "" && t.ID == "" {
		username = "Anonymous"
	} else {
		username = t.Name
	}
	if t.Tripcode != "" {
		if t.Name != "" {
			username = username + " ◆" + t.Tripcode
		} else {
			username = "◆" + t.Tripcode
		}
	}
	if t.Capcode != "" {
		username = username + "!!" + t.Capcode
	}
	if t.ID != "" {
		username = username + " [ID: " + t.ID + "]"
	}
	return username
}

// why moot why
func fudge(b []byte) []byte {
	before := []byte(`{"pages":`)
	after := byte('}')
	return append(append(before, b...), after)
}

func getBytes(url string, shouldFudge bool) (b []byte, statusCode int) {
	resp, _ := http.Get(url)
	b, _ = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if shouldFudge {
		b = fudge(b)
	}
	return b, resp.StatusCode
}

func stringToDocument(data string) *goquery.Document {
	doc, err := html.Parse(strings.NewReader(data))
	if err != nil {
		panic("HTML Error: stringToDocument()")
	}
	return goquery.NewDocumentFromNode(doc)
}
