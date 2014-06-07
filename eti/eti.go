package eti

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"code.google.com/p/cookiejar"
	"code.google.com/p/go.net/html"
	"github.com/PuerkitoBio/goquery"
	"github.com/guregu/bbs"
)

const ETITopicsPerPage = 50.0
const loginURL = "http://iphone.endoftheinter.net/"
const topicsURL = "http://boards.endoftheinter.net/topics/"
const threadURL = "http://boards.endoftheinter.net/showmessages.php?topic="
const archivedThreadURL = "http://archives.endoftheinter.net/showmessages.php?topic="
const postReplyURL = "http://boards.endoftheinter.net/async-post.php"
const postThreadURL = "http://boards.endoftheinter.net/postmsg.php"

const sigSplitHTML = "<br/>\n---<br/>"
const sigSplitText = "\n---\n"
const loginPageTitle = "Das Ende des Internets"

var AllPosts = bbs.Range{1, 5000}
var DefaultRange = bbs.Range{1, 50}

var topicIDExtractor = regexp.MustCompile(`<script type="text\/javascript">onDOMContentLoaded\(function\(\){new QuickPost\(([0-9]+),`)

var Hello = bbs.HelloMessage{
	Command:         "hello",
	Name:            "ETI Relay",
	ProtocolVersion: 0,
	Description:     "End of the Internet -> BBS Relay",
	Options:         []string{"tags", "avatars", "usertitles", "filter", "signatures", "range", "bookmarks"},
	Access: bbs.AccessInfo{
		GuestCommands: []string{"hello", "login", "logout"},
		UserCommands:  []string{"get", "list", "post", "reply", "info"},
	},
	Formats:       []string{"html", "text"},
	Lists:         []string{"thread", "bookmark"},
	ServerVersion: "eti-relay 0.2",
	IconURL:       "/static/eti.png",
	DefaultRange:  DefaultRange,
}

func Setup(name, desc, realtimePath string) {
	Hello.Name = maybe(name, "ETI")
	Hello.Description = maybe(desc, "ETI Gateway")
	Hello.RealtimeURL = realtimePath
}

type ETI struct {
	HTTPClient *http.Client
	Username   string

	loggedIn bool
}

func New() bbs.BBS {
	return new(ETI)
}

func (eti *ETI) grab(url string) (*goquery.Document, error) {
	log.Printf("Getting: [%s] %s", eti.Username, url)
	r, err := eti.HTTPClient.Get(url)
	defer r.Body.Close()
	if err != nil {
		return nil, err
	}
	return goquery.NewDocumentFromReader(r.Body)
}

// grabAjax gets one of those ajaxed }"html goes here" docs
func (eti *ETI) grabAjax(url string) (*goquery.Document, error) {
	log.Printf("Getting: [%s] %s", eti.Username, url)
	r, err := eti.HTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(r.Body)
	switch {
	case len(data) < 3:
		return nil, errors.New("bad response")
	case err != nil:
		return nil, err
	case data[0] != '}':
		return nil, sessionError
	}
	var raw string
	err = json.Unmarshal(data[1:], &raw)
	if err != nil {
		return nil, err
	}
	return goquery.NewDocumentFromReader(strings.NewReader(raw))
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

func (eti *ETI) fetchMetadata(id string) (metadata, error) {
	url := threadURL + id
	doc, err := eti.grab(url)
	if err != nil {
		return metadata{}, err
	}

	md := metadata{
		ID: id,
		Thread: bbs.ThreadMessage{
			Command: "msg",
			ID:      id,
			Format:  "html", // TODO
		},
	}

	// is ETI even ok?
	// if len(doc.Find(".body").Nodes) == 0 {
	// 	return metadata{}, serverIsDownError
	// }

	// did we get logged out?
	if doc.Find("title").Text() == loginPageTitle {
		return metadata{}, sessionError
	}

	// can we even look at this thread?
	if doc.Find(".body > em").Text() == "You are not authorized to view messages on this board." {
		return metadata{}, accessDeniedError
	}

	// topic title
	md.Thread.Title = doc.Find(".body > h1").First().Text()

	// error-y text that shows up under the topic title
	redText := doc.Find(".body > h2 > em").Text()
	switch redText {
	case "This topic has been archived. No additional messages may be posted.":
		md.Archived, md.Thread.Closed = true, true
	case "This topic has been closed. No additional messages may be posted.":
		md.Thread.Closed = true
	}

	// extract the tags
	doc.Find(".body > h2 > div > a").Each(func(i int, s *goquery.Selection) {
		md.Thread.Tags = append(md.Thread.Tags, s.Text())
	})

	// get the last page and estimate the # of posts
	lastPage, err := strconv.Atoi(doc.Find("#u0_2 > span:first-child").Text())
	if err != nil {
		return metadata{}, errors.New("parsing - latspage")
	}
	md.pages, md.Thread.Total = lastPage, lastPage*50

	return md, nil
}

func (eti *ETI) fetchArchivedMsgs(md metadata) (*goquery.Selection, error) {
	var msgs *goquery.Selection
	urls, err := threadURLs(md.Thread.ID, bbs.Range{1, md.Thread.Total}, true)
	if err != nil {
		return nil, err
	}
	for _, url := range urls {
		doc, err := eti.grab(url)
		if err != nil {
			return nil, err
		}

		// did we get logged out?
		if doc.Find("title").Text() == loginPageTitle {
			return nil, sessionError
		}

		m := doc.Find("message-container")
		if msgs == nil {
			msgs = m
		} else {
			msgs = msgs.Union(m)
		}

		if m.Size() < 50 {
			// last page
			break
		}
	}

	// remove mod notes from the archives... RIP
	msgs.Find(".secret").Each(func(i int, sel *goquery.Selection) {
		// hope this works
		sel.Nodes[0].FirstChild = nil
	})

	return msgs, nil
}

func (client *ETI) Get(m bbs.GetCommand) (t bbs.ThreadMessage, err error) {
	if !client.IsLoggedIn() {
		return bbs.ThreadMessage{}, errors.New("session")
	}

	var reqRange = m.Range
	if reqRange.Empty() {
		reqRange = DefaultRange
	} else {
		if !reqRange.Validate() {
			err = errors.New(fmt.Sprintf("Invalid range (%v)", m.Range))
			return
		}
	}
	// tokens have precent over range for now
	// I don't think they should be together anyway
	if m.Token != "" {
		if r, ok := parseToken(m.Token); ok {
			reqRange = r
		}
	}

	// see if we can get the cached version
	// TODO: formatting

	md := getThread(m.ThreadID)
	if md == nil {
		fetch, err := client.fetchMetadata(m.ThreadID)
		if err != nil {
			return bbs.ThreadMessage{}, err
		}
		md = &fetch
	}

	danger := false // can we cache this thread without leaking stuff?
	// if we haven't fetched any messages yet
	if len(md.Thread.Messages) == 0 {
		var msgs *goquery.Selection
		if md.Archived {
			var err error
			msgs, err = client.fetchArchivedMsgs(*md)
			if err != nil {
				return bbs.ThreadMessage{}, err
			}
		} else {
			// get the whole fkn thread
			doc, err := client.grabAjax(fmt.Sprintf(
				"http://boards.endoftheinter.net/moremessages.php?topic=%s&old=0&new=6666&filter=0", m.ThreadID))
			if err != nil {
				return bbs.ThreadMessage{}, err
			}
			msgs = doc.Find(".message-container")

			// did we get logged out?
			if doc.Find("title").Text() == loginPageTitle {
				return bbs.ThreadMessage{}, sessionError
			}
		}

		msgs.Find("a script").Each(transmuteImages)

		// TODO: bbshtmlify
		md.Thread.Messages = parseMessages(msgs, "html")
		md.Thread.Total = len(md.Thread.Messages)
		md.Thread.Range = bbs.Range{1, md.Thread.Total}

		if msgs.Find(".secret").Size() == 0 {
			// don't cache mod notes
			go updateThread(*md)
		} else {
			danger = true
		}

		if reqRange.Start > len(md.Thread.Messages) {
			// since we just got the whole thread we know this request will be empty
			t = md.Thread
			t.Range = reqRange
			t.Messages = []bbs.Message{}
			t.More = false
			t.NextToken = strconv.Itoa(len(md.Thread.Messages))
			return t, nil
		}
	}

	if reqRange.End > len(md.Thread.Messages) {
		doc, err := client.grabAjax(fmt.Sprintf(
			// old=1, new=3
			// gives posts: 2, 3
			"http://boards.endoftheinter.net/moremessages.php?topic=%s&old=%d&new=%d&filter=0", m.ThreadID, len(md.Thread.Messages), reqRange.End))
		if err != nil {
			return bbs.ThreadMessage{}, err
		}
		if doc.Find("title").Text() == loginPageTitle {
			return bbs.ThreadMessage{}, sessionError
		}
		msgs := doc.Find(".message-container")
		msgs.Find("a script").Each(transmuteImages)
		if msgs.Find(".secret").Size() > 0 {
			danger = true
		}
		incoming := parseMessages(msgs, "html")
		md.Thread.Messages = append(md.Thread.Messages, incoming...)
	}

	if !danger {
		go updateThread(*md)
	}

	t = md.Thread
	// filter by poster
	if m.Filter != "" {
		var msgs []bbs.Message
		for _, msg := range t.Messages {
			if msg.AuthorID == m.Filter {
				msgs = append(msgs, msg)
			}
		}
		t.Messages = msgs
		t.Filter = m.Filter
	}
	start, stop := max(reqRange.Start-1, 0), min(reqRange.End, len(t.Messages))
	// filter by range
	t.Messages = t.Messages[start:stop]
	t.Range = bbs.Range{start + 1, stop}
	t.More = stop < len(t.Messages)
	t.NextToken = strconv.Itoa(stop)
	return t, nil
}

func (client *ETI) List(m bbs.ListCommand) (ret bbs.ListMessage, err error) {
	//ETI requires we be logged in to do anything
	if !client.IsLoggedIn() {
		return bbs.ListMessage{}, errors.New("session")
	}

	query := m.Query
	data := getURLData(client.HTTPClient, topicsURL+query)
	doc := stringToDocument(data)

	// TODO: check for 500 whitescreenlinks

	if doc.Find("title").Text() == loginPageTitle {
		return bbs.ListMessage{}, sessionError
	}

	ohs := doc.Find("tr")

	if ohs.Size() == 0 {
		fmt.Println("----- FUCK -----")
		fmt.Println(data)
		return bbs.ListMessage{}, errors.New("No results: " + query)
	}

	// update bookmarks
	findBookmarks(client.Username, doc)

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

	bookmarks := getBookmarks(eti.Username)
	if bookmarks == nil {
		doc := stringToDocument(getURLData(eti.HTTPClient, topicsURL+"LUE"))
		bookmarks = findBookmarks(eti.Username, doc)
	}
	bmm = bbs.BookmarkListMessage{
		Command:   "list",
		Type:      "bookmark",
		Bookmarks: bookmarks,
	}

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

		log.Println("logged in", eti.Username)
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
	b, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	response_html := string(b)
	if resp.StatusCode == 200 {
		// as of 2014, this gives you your topic instead of a 302
		// so we have to check for an error
		doc := stringToDocument(response_html)
		errorText := doc.Find(".body > em")
		if errorText.Size() > 0 {
			return bbs.OKMessage{}, errors.New(errorText.Text())
		}
		// ok!
		id := topicIDExtractor.FindStringSubmatch(response_html)[1]
		return bbs.OKMessage{"ok", "post", id}, nil
	} else if resp.StatusCode == 302 {
		// seems like this doesn't get called anymore :(
		ret := strings.Split(resp.Header.Get("Location"), "?topic=")[1]
		fmt.Println("AHHH", ret)
		return bbs.OKMessage{"ok", "post", ret}, nil
	} else {
		return bbs.OKMessage{}, errors.New("Maybe ETI is down?")
	}
	fmt.Println(resp.StatusCode)
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
	fmt.Println("getting:", url)
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
			username = s.Find(".message-top a").First().Text()
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maybe(test string, def string) string {
	if test == "" {
		return def
	}
	return test
}
