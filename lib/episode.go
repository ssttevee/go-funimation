package funimation

import (
	"errors"
	"net/http"
	"strings"
	"strconv"
	"bytes"
	"fmt"
)

type EpisodeLanguage string

const (
	Dubbed EpisodeLanguage = "dub"
	Subbed                 = "sub"
)

type EpisodeQuality int

const (
	NoQuality EpisodeQuality = iota
	StandardDefinition
	HighDefinition
	FullHighDefinition
)

func (q EpisodeQuality) String() string {
	switch q {
	case StandardDefinition:
		return "480p"
	case HighDefinition:
		return "720p"
	case FullHighDefinition:
		return "1080p"
	}

	return "none"
}

func ParseEpisodeQuality(qualityStr string) EpisodeQuality {
	if qualityStr == "fhd" || qualityStr == "1080p" {
		return FullHighDefinition
	} else if qualityStr == "hd" || qualityStr == "720p" {
		return HighDefinition
	}

	return StandardDefinition
}

type EpisodeType string

const (
	Regular EpisodeType = "Episode"
	Ova                 = "OVA"
	Special             = "Special"
)

type Episode struct {
	seasonNum   int
	episodeNum  float32
	episodeType EpisodeType

	title       string
	summary     string

	videoUrls   map[EpisodeLanguage]map[EpisodeQuality]string

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

func (e *Episode) Type() string {
	return e.episodeType
}

func (e *Episode) TypeCode() string {
	if e.episodeType == Ova {
		return "o"
	} else if e.episodeType == Special {
		return "special"
	}

	return "e"
}

func (e *Episode) Title() (string) {
	return e.title
}

func (e *Episode) Summary() (string) {
	return e.summary
}

func (e *Episode) Languages() ([]EpisodeLanguage) {
	langs := make([]EpisodeLanguage, 0, len(e.videoUrls))

	for lang, _ := range e.videoUrls {
		langs = append(langs, lang)
	}

	return langs
}

func (e *Episode) Qualities(lang EpisodeLanguage) ([]EpisodeQuality) {
	urls := e.videoUrls[lang]
	qualities := make([]EpisodeQuality, 0, len(urls))

	for quality, _ := range urls {
		qualities = append(qualities, quality)
	}

	return qualities
}

func (e *Episode) GetVideoUrl(lang EpisodeLanguage, quality EpisodeQuality) (string, error) {
	if urls, ok := e.videoUrls[lang]; ok {
		if url, ok := urls[quality]; ok {
			if url == "subscriptionLoggedOut" {
				return "", errors.New("This video is members only")
			} else if url == "matureContentLoggedOut" {
				return "", errors.New("This video is members only and you must be at least 17")
			} else if url == "nonSubscription" {
				return "", errors.New("This video is only available to subscribers")
			} else if url == "matureContentLoggedIn" {
				return "", errors.New("You must be at least 17")
			} else if url == "territoryUnavailable" {
				return "", errors.New("This video is not available in your territory")
			}

			return url, nil
		}
	}

	return "", errors.New("No videos found with the given language and quality")
}

func (e *Episode) GuessVideoUrl(lang EpisodeLanguage, quality EpisodeQuality) (string, error) {
	funId, err := e.getFunimationId(lang)
	if err != nil {
		return "", err
	}

	if e.authToken == "" {
		return "", errors.New("Couldn't find auth token")
	}

	if quality == NoQuality {
		return "", errors.New("Quality cannot be none")
	}

	bitrate := 1500
	switch quality {
	case HighDefinition:
		bitrate = 2500
	case FullHighDefinition:
		bitrate = 4000
	}

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
	e.videoUrls = make(map[EpisodeLanguage]map[EpisodeQuality]string)

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

		// collect video urls
		urls := make(map[EpisodeQuality]string)

		if video.sdUrl != "" {
			urls[StandardDefinition] = video.sdUrl
		}
		if video.hdUrl != "" {
			urls[HighDefinition] = video.hdUrl
		}
		if video.hd1080Url != "" {
			urls[FullHighDefinition] = video.hd1080Url
		}

		e.videoUrls[language] = urls
	}

	// collect episode number
	e.episodeNum = clip.number

	return nil
}

func (e *Episode) GetBestQuality(el EpisodeLanguage, onlyAvailable bool) EpisodeQuality {
	quality := NoQuality

	if urls, ok := e.videoUrls[el]; ok {
		for q, url := range urls {
			if onlyAvailable && !strings.HasPrefix(url, "http") {
				continue
			}

			if (q == StandardDefinition && quality != NoQuality) || (q == HighDefinition && quality != NoQuality && quality != StandardDefinition) {
				quality = q
			} else if q == FullHighDefinition {
				quality = q
				break
			}
		}
	}

	return quality
}

type EpisodeList []*Episode

func (e EpisodeList) String() (string) {
	var buf bytes.Buffer

	seasons := make(map[int]EpisodeList)
	for _, ep := range e {
		if _, ok := seasons[ep.seasonNum]; !ok {
			seasons[ep.seasonNum] = make(EpisodeList, 0)
		}

		seasons[ep.seasonNum] = append(seasons[ep.seasonNum], ep)
	}

	fmt.Fprintln(&buf, fmt.Sprintf("%d Seasons, %d Episodes", len(seasons), len(e)))

	for seasonNum, episodes := range seasons {
		fmt.Fprintf(&buf, "\nSeason %d:\n", seasonNum)

		for _, ep := range episodes {
			if ep.episodeNum == 0 {
				fmt.Fprintf(&buf, "\t%s - %s\n", ep.episodeType, ep.title)
			} else {
				fmt.Fprintf(&buf, "\t%s %v - %s\n", ep.episodeType, ep.episodeNum, ep.title)
			}

			for lang := range ep.videoUrls {
				fmt.Fprintf(&buf, "\t\t%sbed: ", lang)

				if qs := ep.Qualities(lang); len(qs) > 0 {
					qNames := make([]string, len(qs))
					for i, q := range qs {
						qNames[i] = q.String()
					}

					fmt.Fprintln(&buf, strings.Join(qNames, ", "))
				} else {
					fmt.Fprintln(&buf, NoQuality.String())
				}
			}
		}
	}

	return string(buf.Bytes())
}