package funimation

import (
	"errors"
	"ssttevee.com/funimation-downloader/funimation/m3u8"
	"net/http"
	"strings"
	"strconv"
	"bytes"
	"fmt"
	"math"
	"ssttevee.com/funimation-downloader/funimation/downloader"
)

type EpisodeLanguage string

const (
	Dubbed EpisodeLanguage = "dub"
	Subbed                 = "sub"
)

type EpisodeType string

const (
	Regular EpisodeType = "Episode"
	Ova                 = "OVA"
)

type Episode struct {
	seasonNum   int
	episodeNum  float32
	episodeType EpisodeType

	title       string
	summary     string

	languages   []EpisodeLanguage
	bitRates    map[EpisodeLanguage][]int

	url         string

	funIds      map[EpisodeLanguage]string
	authToken   string
	client      *http.Client
}

func (e *Episode) SeasonNumber() (int) {
	return e.seasonNum
}

func (e *Episode) EpisodeNumber() (float32) {
	return e.episodeNum
}

func (e *Episode) Title() (string) {
	return e.title
}

func (e *Episode) Summary() (string) {
	return e.summary
}

func (e *Episode) Languages() ([]EpisodeLanguage) {
	return e.languages[:]
}

func (e *Episode) BitRates(lang EpisodeLanguage) ([]int) {
	return e.bitRates[lang][:]
}

func (e *Episode) GetHLSUrl(bitrate int, lang EpisodeLanguage) (string, error) {
	funId, err := e.getFunimationId(lang)
	if err != nil {
		return "", err
	}

	if e.authToken == "" {
		return "", errors.New("Couldn't find auth token")
	}

	bitrate = e.FixBitrate(bitrate, lang)

	return fmt.Sprintf("http://wpc.8c48.edgecastcdn.net/038C48/SV/480/%s/%s-480-%dK.mp4.m3u8%s", funId, funId, bitrate, e.authToken), nil
}

func (e *Episode) GetMp4Url(bitrate int, lang EpisodeLanguage) (string, error) {
	funId, err := e.getFunimationId(lang)
	if err != nil {
		return "", err
	}

	if e.authToken == "" {
		return "", errors.New("Couldn't find auth token")
	}

	bitrate = e.FixBitrate(bitrate, lang)

	return fmt.Sprintf("http://wpc.8c48.edgecastcdn.net/008C48/SV/480/%s/%s-480-%dK.mp4%s", funId, funId, bitrate, e.authToken), nil
}

func (e *Episode) getFunimationId(lang EpisodeLanguage) (string, error) {
	if e.funIds != nil {
		// if there's only one, just return that one regardless of what was requested
		if len(e.funIds) == 1 {
			for _, fid := range e.funIds {
				return fid, nil
			}
		}
		if fid, ok := e.funIds[lang]; ok {
			return fid, nil
		}
		return "", errors.New("episode: lang not found")
	}

	err := e.collectData()
	if err != nil {
		return "", err
	}

	return e.getFunimationId(lang)
}

func (e *Episode) collectData() (error) {
	playersData, err := getPlayersDataFromUrl(e.client, e.url)
	if err != nil {
		return err
	}

	playerData := playersData[0]

	e.funIds = make(map[EpisodeLanguage]string)
	e.bitRates = make(map[EpisodeLanguage][]int)
	e.languages = make([]EpisodeLanguage, 0)

	found := false
	for _, pli := range playerData.playlist {
		err := e.handlePlaylistItem(pli)
		if err == nil {
			found = true
		} else if err != NotFound {
			return err
		}
	}

	if found {
		return nil
	} else {
		return errors.New("episode: video set not found")
	}
}

func (e *Episode) handlePlaylistItem(pi playlistItem) (error) {
	if clip, ok := pi.(*playlistItemClip); ok {
		err := e.handlePlaylistItemClip(clip)
		if err != nil {
			return err
		}

		return nil
	} else if container, ok := pi.(*playlistItemContainer); ok {
		found := false
		for _, item := range container.items {
			err := e.handlePlaylistItem(item)
			if err == nil {
				space := strings.LastIndex(container.title, " ")

				seNum, err := strconv.ParseInt(container.title[space + 1:], 10, 32)
				if err != nil {
					return errors.New("episode: can't parse season number (\"" + container.title[space + 1:] + "\")")
				}

				e.seasonNum = int(seNum)

				found = true
			} else if err != NotFound {
				return err
			}
		}

		if found {
			return nil
		} else {
			return NotFound
		}
	}

	return NotFound
}

func (e *Episode) handlePlaylistItemClip(clip *playlistItemClip) (error) {
	const dash = " - "

	if clip.videoSet == nil || len(clip.videoSet) == 0 {
		return NotFound
	}

	e.title = clip.title

	titleDash := strings.Index(e.title, dash)
	if titleDash != -1 {
		e.title = e.title[titleDash + len(dash):]
	}

	e.summary = clip.description

	for _, video := range clip.videoSet {
		language := video.languageMode

		// collect auth token
		e.authToken = video.authToken

		// collect funimation id
		e.funIds[language] = video.funimationId

		// collect bitrates
		e.bitRates[language] = make([]int, 0)

		if strings.HasPrefix(video.sdUrl, "http") {
			e.bitRates[language] = append(e.bitRates[language], 750, 1500)
		}
		if strings.HasPrefix(video.hdUrl, "http") {
			e.bitRates[language] = append(e.bitRates[language], 2000, 2500)
		}
		if strings.HasPrefix(video.hd1080Url, "http") {
			e.bitRates[language] = append(e.bitRates[language], 4000)
		}

		// collect languages
		e.languages = append(e.languages, language)
	}

	// collect episode number
	e.episodeNum = clip.number

	return nil
}

func (e *Episode) GetDownloader(bitRate int, language EpisodeLanguage) (*downloader.Downloader, error) {
	mp4Url, err := e.GetMp4Url(bitRate, language)
	if err != nil {
		return nil, err
	}

	stream, err := downloader.New(mp4Url)
	if err != nil {
		return nil, err
	}

	return stream, nil
}

func (e *Episode) GetStream(bitRate int, language EpisodeLanguage) (*m3u8.M3u8, error) {
	streamUrl, err := e.GetHLSUrl(bitRate, language)
	if err != nil {
		return nil, err
	}

	stream, err := m3u8.New(streamUrl)
	if err != nil {
		return nil, err
	}

	return stream, nil
}

func (e *Episode) FixBitrate(bitrate int, lang EpisodeLanguage) (int) {
	if bitrate == 0 {
		for _, br := range e.bitRates[lang] {
			if br > bitrate {
				bitrate = br
			}
		}

		return bitrate
	} else {
		for _, br := range e.bitRates[lang] {
			if br == bitrate {
				return bitrate
			}
		}

		closest := bitrate
		for _, br := range e.bitRates[lang] {
			if math.Abs(float64(bitrate - closest)) > math.Abs(float64(bitrate - br)) {
				closest = br
			}
		}

		return closest
	}
}

type EpisodeList []*Episode

func (e EpisodeList) String() (string) {
	buf := &bytes.Buffer{}

	seasons := make(map[int]EpisodeList)
	for _, ep := range e {
		if _, ok := seasons[ep.seasonNum]; !ok {
			seasons[ep.seasonNum] = make(EpisodeList, 0)
		}

		seasons[ep.seasonNum] = append(seasons[ep.seasonNum], ep)
	}

	fmt.Fprintln(buf, fmt.Sprintf("%d Seasons, %d Episodes", len(seasons), len(e)))

	for seasonNum, episodes := range seasons {
		fmt.Fprintln(buf)
		fmt.Fprintln(buf, fmt.Sprintf("Season %d:", seasonNum))

		for _, ep := range episodes {
			fmt.Fprintln(buf, fmt.Sprintf("  Episode %v - %s", ep.episodeNum, ep.title))
			fmt.Fprintln(buf, fmt.Sprintf("    Bitrates: %v", ep.bitRates))
			fmt.Fprintln(buf, fmt.Sprintf("    Language Modes: %v", ep.languages))
		}
	}

	return string(buf.Bytes())
}