package funimation

import (
	"net/http"
	"fmt"
	"io"
	"bufio"
	"github.com/hishboy/gocommons/lang"
	"errors"
	"encoding/json"
	"strings"
	"strconv"
)

type playerData struct {
	showSlug string
	playlist []playlistItem
}

type playlistItem interface {
	IAmAPlaylistItem()
}

type basePlaylistItem struct {
	description string
	itemClass   string
	itemSlug    string
	itemType    string
	showName    string
	showId      int
	showUrl     string
	title       string
}

type playlistItemContainer struct {
	basePlaylistItem
	items []playlistItem
}

func (x *playlistItemContainer) IAmAPlaylistItem() {
	// do nothing
}

type playlistItemClip struct {
	basePlaylistItem
	videoSet []*videoItem
	number   float32
}

func (x *playlistItemClip) IAmAPlaylistItem() {
	// do nothing
}

type videoItem struct {
	funimationId string
	authToken    string
	hd1080Url    string
	hdUrl        string
	languageMode EpisodeLanguage
	sdUrl        string
}

func getPlayersDataFromUrl(client *http.Client, url string) ([]*playerData, error) {
	if !strings.HasPrefix(url, "http://www.funimation.com") {
		return nil, errors.New("Url not supported: " + url)
	}

	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("playersData: got status code %d from %s", res.StatusCode, url))
	}

	jsonBytes, err := isolatePlayersDataJson(res.Body)
	if err != nil {
		return nil, err
	}

	return getPlayersData(jsonBytes)
}

func isolatePlayersDataJson(rd io.Reader) ([]byte, error) {
	r := bufio.NewReader(rd)

	stack := lang.NewStack()

	needle := []byte("var playersData = ")
	matches := 0

	jsonBytes := make([]byte, 0, 1024)

	escaped := false
	inquote := false
	for {
		b, err := r.ReadByte()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		if matches < len(needle) {
			if b == needle[matches] {
				matches++
			} else {
				matches = 0
			}
		} else {
			if inquote {
				if escaped {
					escaped = false
				} else if b == '\\' {
					escaped = true
				} else if b == '"' {
					inquote = false
				}
			} else if b == '"' {
				inquote = true
			} else if b == '[' || b == '{' {
				stack.Push(b)
			} else if b == ']' {
				if stack.Peek().(byte) == '[' {
					stack.Pop()
				} else {
					// syntax error...
					return nil, errors.New("playersdata has syntax error")
				}
			} else if b == '}' {
				if stack.Peek().(byte) == '{' {
					stack.Pop()
				} else {
					// syntax error...
					return nil, errors.New("playersdata has syntax error")
				}
			} else if b == ';' && stack.Len() == 0 {
				// done!
				break
			}

			jsonBytes = append(jsonBytes, b)
		}
	}

	return jsonBytes, nil
}

func getPlayersData(b []byte) ([]*playerData, error) {
	var dst []map[string]interface{}

	err := json.Unmarshal(b, &dst)
	if err != nil {
		return nil, err
	}

	ret := make([]*playerData, 0, 1)
	for _, pd := range dst {
		dat := &playerData{}

		if selectedItemAK, ok := pd["selectedItemAK"]; ok {
			dat.showSlug = selectedItemAK.(string)
		}

		dat.playlist = make([]playlistItem, 0)
		if playlist, ok := pd["playlist"]; ok {
			for _, pli := range playlist.([]interface{}) {
				plItem, err := formatPlaylistItem(pli.(map[string]interface{}))
				if err == NotFound {
					continue
				} else if err != nil {
					return nil, err
				}

				dat.playlist = append(dat.playlist, plItem)
			}
		}

		ret = append(ret, dat)
	}

	return ret, nil
}

func formatPlaylistItem(m map[string]interface{}) (playlistItem, error) {
	var ret playlistItem
	var bas *basePlaylistItem
	if itemType, ok := m["itemType"]; ok {
		if itemType.(string) == "container" {
			out, err := formatPlaylistItemContainer(m)
			if err != nil {
				return nil, err
			}
			ret = out
			bas = &out.(*playlistItemContainer).basePlaylistItem
		} else if itemType.(string) == "clip" {
			out, err := formatPlaylistItemClip(m)
			if err != nil {
				return nil, err
			}
			ret = out
			bas = &out.(*playlistItemClip).basePlaylistItem
		} else {
			return nil, NotFound
		}

		bas.itemType = itemType.(string)
	}

	if desc, ok := m["description"]; ok {
		bas.description = desc.(string)
	}

	if icls, ok := m["itemClass"]; ok {
		bas.itemClass = icls.(string)
	}

	if ak, ok := m["itemAK"]; ok {
		bas.itemSlug = ak.(string)
	}

	if artist, ok := m["artist"]; ok {
		bas.showName = artist.(string)
	}

	if url, ok := m["showUrl"]; ok {
		lastSlash := strings.LastIndex(url.(string), "/")
		if lastSlash != -1 {
			bas.showUrl = url.(string)[lastSlash + 1:]
		}

		bas.showUrl = url.(string)
	}

	if nom, ok := m["title"]; ok {
		bas.title = nom.(string)
	}

	return ret, nil
}

func formatPlaylistItemClip(m map[string]interface{}) (playlistItem, error) {
	ret := playlistItemClip{}

	if _, ok := m["videoSet"]; !ok {
		return nil, NotFound
	}

	ret.videoSet = make([]*videoItem, 0)
	for _, v := range m["videoSet"].([]interface{}) {
		video := v.(map[string]interface{})
		vi := videoItem{}

		if at, ok := video["authToken"]; ok {
			vi.authToken = at.(string)
		}

		if fid, ok := video["FUNImationID"]; ok {
			vi.funimationId = fid.(string)
		}

		if url, ok := video["hdUrl"]; ok {
			vi.hdUrl = url.(string)
		}

		if url, ok := video["hd1080Url"]; ok {
			vi.hd1080Url = url.(string)
		}

		if url, ok := video["sdUrl"]; ok {
			vi.sdUrl = url.(string)
		}

		if lm, ok := video["languageMode"]; ok {
			vi.languageMode = EpisodeLanguage(lm.(string))
		}

		ret.videoSet = append(ret.videoSet, &vi)
	}

	if len(ret.videoSet) == 0 {
		return nil, NotFound
	}

	if n, ok := m["number"]; ok {
		num, err := strconv.ParseFloat(n.(string), 32)
		if err != nil {
			return nil, err
		}

		ret.number = float32(num)
	}

	return &ret, nil
}

func formatPlaylistItemContainer(m map[string]interface{}) (playlistItem, error) {
	ret := playlistItemContainer{}

	ret.items = make([]playlistItem, 0)
	if itemsMap, ok := m["items"]; ok {
		for _, item := range itemsMap.([]interface{}) {
			pli, err := formatPlaylistItem(item.(map[string]interface{}))
			if err == NotFound {
				continue
			} else if err != nil {
				return nil, err
			}

			ret.items = append(ret.items, pli)
		}
	}

	if len(ret.items) == 0 {
		return nil, NotFound
	}

	return &ret, nil
}