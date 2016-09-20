package funimation // import "golang.ssttevee.com/funimation/lib"

import (
	"net/http/cookiejar"
	"net/http"
	"net/url"
	"errors"
	"strconv"
	"fmt"
	"math/rand"
)
var NotFound = errors.New("Not found")

const uaFmt = "Mozilla/%.1f (Linux; %s; Android %.1f.%d; en-us) AppleWebkit/%.2f (KHTML, like Gecko) Version/%.1f Mobile Safari/%.2f"
var mobileUA string

func init() {
	RegenerateUA()
}

type Client struct {
	httpClient *http.Client
}

func RegenerateUA() {
	mobileUA = fmt.Sprintf(uaFmt,
		rand.Float32() * float32(10), // mozilla version
		[]string{"N", "U", "I"}[rand.Intn(3)], // security strength
		rand.Float32() * float32(10), // android major and minor version
		rand.Intn(10), // android revision number
		rand.Float32() * float32(1000), // webkit version
		rand.Float32() * float32(10), // safari version
		rand.Float32() * float32(1000)) // safari build number
}

func New(cookieJar *cookiejar.Jar) (*Client) {
	return &Client{
		httpClient: &http.Client{
			Jar: cookieJar,
		},
	}
}

func (f *Client) Login(email, password string) error {
	data := map[string][]string{
		"email_field":{
			email,
		},
		"password_field":{
			password,
		},
	}

	res, err := f.httpClient.PostForm("http://www.funimation.com/login", url.Values(data))
	if err != nil {
		return err
	}

	if res.Header.Get("Location") == "http://www.funimation.com/login" {
		return errors.New("Login fail")
	}

	return nil
}

func (f *Client) GetSeries(showSlug string) (*Series, error) {
	return f.getShowApi("funimation_website", showSlug)
}

func (f *Client) GetSeriesById(showId int) (*Series, error) {
	return f.getShowApi("show_id", showId)
}

func (f *Client) getShowApi(param string, value interface{}) (*Series, error) {
	ajax, err := getJsonObject(f.httpClient, fmt.Sprintf("http://www.funimation.com/frontend_api/getShow/%s/%v", param, value))
	if err != nil {
		return nil, err
	}

	if status, ok := ajax["status"]; !ok || !status.(bool) {
		return nil, NotFound
	}

	info := ajax["info"].(map[string]interface{})

	var showId int
	if id, ok := info["show_id"]; ok {
		num, err := strconv.ParseInt(id.(string), 10, 64)
		if err != nil {
			return nil, err
		}

		showId = int(num)
	} else {
		return nil, NotFound
	}

	title, ok := info["title"]
	if !ok {
		return nil, NotFound
	}

	summary, ok := info["vod_summary_400"]
	if !ok {
		return nil, NotFound
	}

	thumbnail, ok := info["show_thumbnail"]
	if !ok {
		return nil, NotFound
	}

	showSlug, ok := info["funimation_website"]
	if !ok {
		return nil, NotFound
	}

	return &Series{
		slug: showSlug.(string),
		client: f.httpClient,
		showId: showId,
		name: title.(string),
		description: summary.(string),
		posterUrl: "http://www.funimation.com/admin/uploads/default/shows/show_thumbnail/2_thumbnail/" + thumbnail.(string),
	}, nil
}

func (f *Client) GetEpisodeFromUrl(episodeUrl string) (*Episode, error) {
	ep := &Episode{
		client: f.httpClient,
		url: episodeUrl,
	}

	err := ep.collectData()
	if err != nil {
		return nil, err
	}

	return ep, nil
}
