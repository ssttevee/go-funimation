package funimation

import (
	"net/http"
	"fmt"
)

type Series struct {
	showId      int

	name        string
	description string
	posterUrl   string

	slug        string
	episodes    EpisodeList
	client      *http.Client
}

func (s *Series) ShowId() (int) {
	return s.showId
}

func (s *Series) Title() (string) {
	return s.name
}

func (s *Series) Description() (string) {
	return s.description
}

func (s *Series) PosterUrl() (string) {
	return s.name
}

func (s *Series) GetEpisode(ep int) (*Episode, error) {
	if s.episodes != nil {
		if len(s.episodes) < ep{
			return nil, NotFound
		}

		episode := s.episodes[ep - 1]
		return episode, nil
	}

	eps, err := searchForEpisodes(s.client, s.showId, 1, ep - 1)
	if err != nil {
		return nil, err
	}

	if len(eps) > 0 {
		return eps[0], nil
	} else {
		return nil, NotFound
	}
}

func (s *Series) GetEpisodeBySlug(episodeSlug string) (*Episode, error) {
	ep := &Episode{
		client: s.client,
		url: fmt.Sprintf("http://www.funimation.com/shows/%s/videos/official/%s", s.slug, episodeSlug),
	}

	err := ep.collectData()
	if err != nil {
		return nil, err
	}

	return ep, nil
}

func (s *Series) GetEpisodesRange(start, end int) (EpisodeList, error) {
	if s.episodes != nil {
		return s.episodes, nil
	}

	eps, err := searchForEpisodes(s.client, s.showId, end - start + 1, start - 1)
	if err != nil {
		return nil, err
	}

	s.episodes = EpisodeList(eps)

	return s.episodes, nil
}

func (s *Series) GetAllEpisodes() (EpisodeList, error) {
	if s.episodes != nil {
		return s.episodes, nil
	}

	eps, err := searchForEpisodes(s.client, s.showId, int(^uint32(0) >> 1), 0)
	if err != nil {
		return nil, err
	}

	s.episodes = EpisodeList(eps)

	return s.episodes, nil
}