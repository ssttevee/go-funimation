package funimation

import (
	"net/http"
	"bytes"
	"io"
	"encoding/json"
	"fmt"
	"strings"
	"golang.org/x/net/html"
	"sync"
)

var collectCookies sync.Once

func getJsonObject(client *http.Client, url string) (map[string]interface{}, error) {
	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		return nil, NotFound
	}

	buf := &bytes.Buffer{}
	io.Copy(buf, res.Body)

	var ajax map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &ajax); err != nil {
		return nil, err
	}

	return ajax, nil
}

func searchForEpisodes(client *http.Client, showId, limit, offset int) ([]*Episode, error) {
	// collect cookies for the first time
	collectCookies.Do(func() {
		client.Get("http://www.funimation.com/videos/episodes")
	})

	var episodes []*Episode

	searchUrl := fmt.Sprintf("http://www.funimation.com/shows/viewAllFiltered?section=episodes&limit=%d&offset=%d&showid=%d", limit, offset, showId)
	ajax, err := getJsonObject(client, searchUrl)
	if err != nil {
		return nil, err
	}

	tokenizer := html.NewTokenizer(strings.NewReader(ajax["main"].(string)))

	errChan := make(chan error)

	var lookForEpisodeType bool
	var prevEpisode *Episode
	for {
		tokenType := tokenizer.Next()

		if tokenType == html.ErrorToken {
			break
		} else if tokenType == html.StartTagToken {
			token := tokenizer.Token()

			if token.Data == "a" {
				href := ""
				foundWatchLink := false
				for _, attr := range token.Attr {
					if attr.Key == "class" {
						if strings.Contains(attr.Val, "watchLinks") {
							foundWatchLink = true
							lookForEpisodeType = true
						}
					} else if attr.Key == "href" {
						href = attr.Val
					}
				}

				if foundWatchLink {
					ep := &Episode{
						client: client,
						url: href,
						episodeType: Regular}

					episodes = append(episodes, ep)

					go func() {
						errChan<- ep.collectData()
					}()

					prevEpisode = ep
				}
			}
		} else if lookForEpisodeType {
			token := tokenizer.Token()

			if tokenType == html.EndTagToken && token.Data == "a" {
				lookForEpisodeType = false;
			} else if tokenType == html.TextToken {
				text := strings.ToLower(token.Data)
				if strings.Contains(text, "episode") {
					lookForEpisodeType = false;
				} else if strings.Contains(text, "ova") {
					prevEpisode.episodeType = Ova
				} else if strings.Contains(text, "special") {
					prevEpisode.episodeType = Special
				}
			}
		}
	}

	for i := 0; i < len(episodes); i++ {
		if err := <-errChan; err != nil {
			return nil, err
		}
	}

	return episodes, nil
}
