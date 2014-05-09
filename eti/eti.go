package eti

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"code.google.com/p/cookiejar"
	"code.google.com/p/go.net/html"
	"code.google.com/p/go.net/html/atom"
	"github.com/PuerkitoBio/goquery"
	"github.com/guregu/bbs"
)

const ETITopicsPerPage = 50.0
const loginURL = "http://iphone.endoftheinter.net/"
const topicsURL = "http://boards.endoftheinter.net/topics/"
const threadURL = "http://boards.endoftheinter.net/showmessages.php?topic="
const postReplyURL = "http://boards.endoftheinter.net/async-post.php"
const postThreadURL = "http://boards.endoftheinter.net/postmsg.php"

const sigSplitHTML = "<br/>\n---<br/>"
const sigSplitText = "\n---\n"

var AllPosts = bbs.Range{1, 5000}
var DefaultRange = bbs.Range{1, 50}

var Hello = bbs.HelloMessage{
	Command:         "hello",
	Name:            "ETI Relay",
	ProtocolVersion: 0,
	Description:     "End of the Internet -> BBS Relay",
	Options:         []string{"tags", "avatars", "usertitles", "filter", "signatures", "range"},
	Access: bbs.AccessInfo{
		GuestCommands: []string{"hello", "login", "logout"},
		UserCommands:  []string{"get", "list", "post", "reply", "info"},
	},
	Formats:       []string{"html", "text"},
	Lists:         []string{"thread"},
	ServerVersion: "eti-relay 0.1",
	IconURL:       "/static/eti.png",
	DefaultRange:  DefaultRange,
}

type ETI struct {
	HTTPClient *http.Client
	Username   string

	loggedIn bool
}

func New() bbs.BBS {
	return new(ETI)
}

func (eti *ETI) Hello() bbs.HelloMessage {
	return Hello
}

func (eti *ETI) Register(m bbs.RegisterCommand) (okm bbs.OKMessage, err error) {
	err = errors.New("Registration is not supported.")
	return
}

func (eti *ETI) IsLoggedIn() bool {
	return eti.loggedIn
}

func (client *ETI) Get(m bbs.GetCommand) (t bbs.ThreadMessage, err error) {
	//ETI requires we be logged in to do anything
	if !client.IsLoggedIn() {
		return bbs.ThreadMessage{}, errors.New("session")
	}

	if m.Filter == "" {
		//m.Filter = "0" //ETI kludge
	}
	if m.Range == (bbs.Range{}) {
		if m.Token != "" {
			start, err := strconv.Atoi(m.Token)
			if err != nil {
				m.Range = DefaultRange
			} else {
				m.Range = bbs.Range{start, start + DefaultRange.End}
			}
		} else {
			m.Range = DefaultRange
		}
	} else if !m.Range.Validate() {
		err = errors.New(fmt.Sprintf("Invalid range (%v)", m.Range))
		return
	}

	t = bbs.ThreadMessage{
		Command: "msg",
		ID:      m.ThreadID,
		Range:   m.Range,
		Filter:  m.Filter,
		Format:  m.Format,
	}
	var doc *goquery.Document
	var messagesSelection *goquery.Selection
	archived := false
	fmt.Printf("%#v\n", m.Range)
	startPage, _ := etiPages(m.Range)
	//archives doesn't properly redirect so there's some shit here to deal w/ that
	for i, fetchURL := range etiURLs(t) {
		if archived {
			fetchURL = strings.Replace(fetchURL, "boards.endoftheinter.net", "archives.endoftheinter.net", -1)
		}
		currentDoc := stringToDocument(getURLData(client.HTTPClient, fetchURL))
		//this is really ugly but basically archives are broken sorry
		if i == 0 && currentDoc.Find("h2 em").Text() == "This topic has been archived. No additional messages may be posted." {
			archived = true
			fetchURL = strings.Replace(fetchURL, "boards.endoftheinter.net", "archives.endoftheinter.net", -1)
			//it redirects to the first page, so we need to re-do our shit
			if startPage != 1 {
				currentDoc = stringToDocument(getURLData(client.HTTPClient, fetchURL))
			}
			t.Closed = true
		} else if i == 0 && currentDoc.Find("h2 em").Text() == "This topic has been closed. No additional messages may be posted." {
			t.Closed = true
		}

		find := currentDoc.Find(".message-container")
		if find.Size() == 0 {
			break
		}
		if doc == nil {
			doc = currentDoc
			messagesSelection = find
		} else {
			//add posts to one big selection
			messagesSelection = messagesSelection.Union(find)
		}
	}

	if messagesSelection == nil || messagesSelection.Size() == 0 {
		if doc != nil {
			dump, _ := doc.Html()
			fmt.Println(dump)
		}
		return bbs.ThreadMessage{}, errors.New(fmt.Sprintf("Invalid thread: %s. No messages.", m.ThreadID))
	}

	t.Title = doc.Find("h1").First().Text()

	//we get all 50 posts per page, but sometimes we don't want them all
	//there is probably a better way to do this
	skipFirst := m.Range.Start%int(ETITopicsPerPage) - 1
	if skipFirst < 0 {
		skipFirst = 0
	}
	for n_i, node := range messagesSelection.Nodes {
		if n_i < skipFirst {
			messagesSelection = messagesSelection.NotNodes(node)
		} else if n_i > m.Range.End-m.Range.Start+skipFirst {
			messagesSelection = messagesSelection.NotNodes(node)
		}
	}

	//fix image links
	messagesSelection.Find("a script").Each(func(i int, sel *goquery.Selection) {
		a := sel.Parent()
		url, ok := a.Attr("imgsrc")
		if ok {
			for a_i := range a.Nodes[0].Attr {
				if a.Nodes[0].Attr[a_i].Key == "href" {
					a.Nodes[0].Attr[a_i].Val = url
				}
			}
			//transmute script into an image
			//cant believe this works
			script := sel.Nodes[0]
			script.Type = html.ElementNode
			script.Data = "img"
			script.DataAtom = atom.Image
			script.FirstChild = nil
			script.Attr = []html.Attribute{html.Attribute{"", "src", url}}
		}
	})

	tagElems := doc.Find("h2 div a").Not("h2 div span a")
	tags := make([]string, tagElems.Size())
	tagElems.Each(func(t_i int, t_sel *goquery.Selection) {
		tags[t_i] = t_sel.Text()
	})
	t.Tags = tags

	t.Messages = parseMessages(messagesSelection, m.Format)
	t.More = true
	t.NextToken = strconv.Itoa(m.Range.Start + len(t.Messages))
	return t, nil
}

func (client *ETI) List(m bbs.ListCommand) (ret bbs.ListMessage, err error) {
	//ETI requires we be logged in to do anything
	if !client.IsLoggedIn() {
		return bbs.ListMessage{}, errors.New("session")
	}

	query := m.Query
	doc := stringToDocument(getURLData(client.HTTPClient, topicsURL+query))
	ohs := doc.Find("tr")

	if ohs.Size() == 0 {
		return bbs.ListMessage{}, errors.New("No results: " + query)
	}

	var threads []bbs.ThreadListing
	ohs.Each(func(i int, s *goquery.Selection) {
		sel := s.Find(".oh")
		if sel.Size() < 1 {
			return
		}
		link := sel.Find(".fl a")
		href, _ := link.Attr("href")
		id := strings.Split(href, "?topic=")[1]
		title := link.Text()
		sticky := false
		closed := false
		if link.ParentFiltered(".closed").Size() > 0 {
			closed = true
		}
		tag_e := sel.Find(".fr a")
		tags := make([]string, tag_e.Size())
		tag_e.Each(func(t_i int, t_sel *goquery.Selection) {
			tags[t_i] = t_sel.Text()
			if t_sel.Text() == "Pinned" {
				sticky = true
			}
		})
		user_link := s.Find("td:nth-child(2) a")
		username := user_link.Text()
		user_href, href_ok := user_link.Attr("href")
		var userid string
		if !href_ok {
			username = "Anonymous"
			userid = "-1"
		} else {
			userid = strings.Split(user_href, "?user=")[1]
		}
		posts, _ := strconv.Atoi(strings.Fields(s.Find("td:nth-child(3)").Text())[0])
		new_posts := 0
		update_sel := s.Find("td:nth-child(3) span a")
		if update_sel.Size() > 0 {
			new_posts, _ = strconv.Atoi(strings.Trim(update_sel.Text(), "x+"))
		}
		date := s.Find("td:nth-child(4)").Text()

		threads = append(threads, bbs.ThreadListing{
			ID:          id,
			Title:       title,
			Author:      username,
			AuthorID:    userid,
			Date:        date,
			PostCount:   posts,
			Tags:        tags,
			Sticky:      sticky,
			Closed:      closed,
			UnreadPosts: new_posts,
		})
	})
	ret = bbs.ListMessage{
		Command: "list",
		Type:    "thread",
		Query:   query,
		Threads: threads}
	return ret, nil
}

func (eti *ETI) BookmarkList(m bbs.ListCommand) (bmm bbs.BookmarkListMessage, err error) {
	//ETI requires we be logged in to do anything
	if !eti.IsLoggedIn() {
		err = errors.New("session")
		return
	}

	bmm = bbs.BookmarkListMessage{
		Command:   "list",
		Type:      "bookmark",
		Bookmarks: []bbs.Bookmark{},
	}

	doc := stringToDocument(getURLData(eti.HTTPClient, topicsURL+"LUE"))
	doc.Find("#bookmarks span").Each(func(i int, s *goquery.Selection) {
		a := s.Find("a").First()
		href, _ := a.Attr("href")
		if href != "#" {
			split := strings.Split(href, "/")
			query := split[len(split)-1]
			name := a.Text()
			if name != "[edit]" {
				bmm.Bookmarks = append(bmm.Bookmarks, bbs.Bookmark{
					Name:  name,
					Query: query,
				})
			}
		}
	})

	return bmm, nil
}

func (eti *ETI) LogIn(m bbs.LoginCommand) bool {
	username := m.Username
	password := m.Password
	jar, _ := cookiejar.New(nil)
	c := &http.Client{nil, nil, jar}
	resp, _ := c.PostForm(loginURL, url.Values{
		"username": {username},
		"password": {password},
	})
	b, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if string(b) == `<script>document.location.href="/";</script>` {
		//success
		eti.loggedIn = true
		eti.HTTPClient = c
		eti.Username = username
	} else {
		eti.loggedIn = false
	}
	return eti.loggedIn
}

func (eti *ETI) LogOut(m bbs.LogoutCommand) bbs.OKMessage {
	//ok sure
	eti.loggedIn = false
	return bbs.OKMessage{"ok", "logout", ""}
}

func (client *ETI) Post(m bbs.PostCommand) (okm bbs.OKMessage, err error) {
	//ETI requires we be logged in to do anything
	if !client.IsLoggedIn() {
		err = errors.New("session")
		return
	}

	title := m.Title
	tags := m.Tags
	msg := m.Text
	//we need to get the 'h' (hash?) and sig from postmsg.php
	doc := stringToDocument(getURLData(client.HTTPClient, postThreadURL))
	h, _ := doc.Find("input[name='h']").Attr("value")
	//sig is the last (hopefully, only) textarea on the page
	sig := doc.Find("textarea").Last().Text()

	tags_string := ""
	if len(tags) > 0 {
		tags_string = strings.Join(tags, ",")
	}

	v := url.Values{}
	v.Set("title", title)
	v.Set("message", msg+sig)
	v.Set("h", h)
	v.Set("tag", tags_string)
	v.Set("submit", "Post Message")

	resp, _ := client.HTTPClient.PostForm(postThreadURL, v)
	if resp.StatusCode == 200 {
		err = errors.New("Check the length of your title/post.")
	} else if resp.StatusCode == 302 {
		ret := strings.Split(resp.Header.Get("Location"), "?topic=")[1]
		okm = bbs.OKMessage{"ok", "post", ret}
	} else {
		err = errors.New("Maybe ETI is down?")
	}
	return
}

func (client *ETI) Reply(m bbs.ReplyCommand) (okm bbs.OKMessage, err error) {
	//ETI requires we be logged in to do anything
	if !client.IsLoggedIn() {
		err = errors.New("session")
		return
	}

	threadID := m.To
	msg := m.Text
	//we need to get the 'h' (hash?) and sig from the topic
	doc := stringToDocument(getURLData(client.HTTPClient, threadURL+threadID))
	if doc.Find("textarea").Size() == 0 {
		return bbs.OKMessage{}, errors.New("You can't reply to this topic. It's probably archived")
	}

	if doc.Find("h2 em").Text() == "This topic has been closed. No additional messages may be posted." {
		//closed topic
		return bbs.OKMessage{}, errors.New("This topic has been closed. No additional messages may be posted.")
	}

	h, _ := doc.Find("input[name='h']").Attr("value")
	//sig is the last (hopefully, only) textarea on the page (quickpost)
	sig := doc.Find("textarea").Last().Text()

	v := url.Values{}
	v.Set("topic", threadID)
	v.Set("h", h)
	v.Set("message", msg+sig)
	v.Set("-ajaxCounter", "1") //no idea what this is

	resp, _ := client.HTTPClient.PostForm(postReplyURL, v)
	b, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	response_text := string(b)

	if len(response_text) < 2 {
		err = errors.New("Maybe ETI is down?")
		return
	}

	//ghetto error check
	if response_text[1] == '"' {
		errorMessage := strings.Split(response_text, `"`)[1]
		err = errors.New(errorMessage)
		return
	}

	return bbs.OKMessage{"ok", "reply", ""}, nil
}

func getURLData(client *http.Client, url string) string {
	resp, e := client.Get(url)
	if e != nil {
		return "error"
	}
	b, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	return string(b)
}

func stringToDocument(data string) *goquery.Document {
	doc, err := html.Parse(strings.NewReader(data))
	if err != nil {
		panic("HTML Error: stringToDocument()")
	}
	return goquery.NewDocumentFromNode(doc)
}

func parseMessages(messages *goquery.Selection, format string) []bbs.Message {
	ret := make([]bbs.Message, messages.Size())
	messages.Each(func(i int, s *goquery.Selection) {
		msg_id, _ := s.Attr("id")
		msg_header := strings.Fields(s.Find(".message-top").Text())
		var userid, username string
		anon := false
		profileURL, _ := s.Find(".message-top a").First().Attr("href")
		if strings.Contains(profileURL, "showmessages.php") {
			//whoops, this topic is anon
			anon = true
		}
		if !anon {
			userid = strings.Split(profileURL, "?user=")[1]
			username = s.Find(".message-top a:nth-child(2)").Text()
		} else {
			userid = "-" + msg_header[2][1:]
			username = fmt.Sprintf("%s %s", msg_header[1], msg_header[2])
		}
		offset := strings.Count(username, " ") //people with spaces in their name fuck everything up
		date := fmt.Sprintf("%s %s %s", msg_header[offset+4], msg_header[offset+5], msg_header[offset+6])
		message := s.Find(".message")
		usertitle, _ := s.Find(".userpic center").Html()
		userpicscript := s.Find(".userpic-holder script")
		userpicURL := ""

		if userpicscript.Size() > 0 {
			js := userpicscript.Text()
			url := strings.SplitAfter(strings.Split(js, `\/\/`)[1], `",`)[0]
			userpicURL = "http://" + strings.Replace(url, `\/`, "/", -1)
			userpicURL = userpicURL[:len(userpicURL)-2] //chop off the last \, hacky
			//but this whole thing is a hack so idgaf
		}
		var text, sig string
		html, _ := message.Html()
		hasSig := hasSig(html)
		switch format {
		case "text":
			text = message.Text()
			if hasSig {
				text, sig = findSig(text, sigSplitText)
			}
		case "html":
			text = html
			if hasSig {
				text, sig = findSig(html, sigSplitHTML)
			}
		default:
			text = message.Text()
			if hasSig {
				text, sig = findSig(text, sigSplitText)
			}
		}
		ret[i] = bbs.Message{
			ID:                 msg_id,
			Author:             username,
			AuthorID:           userid,
			AuthorTitle:        usertitle,
			AvatarThumbnailURL: userpicURL,
			Date:               date,
			Text:               text,
			Signature:          sig,
		}
	})
	return ret
}

func hasSig(s string) bool {
	split := strings.Split(s, sigSplitHTML)
	return len(split) > 1
}

func findSig(s string, splitter string) (text string, sig string) {
	split := strings.Split(s, splitter)
	length := len(split)
	if length == 1 {
		return s, ""
	}
	text = strings.Join(split[0:length-1], splitter)
	sig = split[length-1]
	return text, sig
}

func etiPages(r bbs.Range) (startPage, endPage int) {
	startPage = int(math.Floor(float64(r.Start)/ETITopicsPerPage) + 1)
	endPage = int(math.Floor(float64(r.End)/ETITopicsPerPage) + 1)
	return
}

func etiURLs(t bbs.ThreadMessage) []string {
	startPage, endPage := etiPages(t.Range)
	if startPage > endPage || endPage-startPage > 500 {
		panic("Invalid range parameters.")
	}
	var ret []string
	for i := int(startPage); i <= int(endPage); i++ {
		ret = append(ret, fmt.Sprintf("http://boards.endoftheinter.net/showmessages.php?topic=%s&page=%d&u=%s", t.ID, i, t.Filter))
	}
	return ret
}
