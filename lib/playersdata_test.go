package funimation

import (
	"testing"
	"fmt"
	"net/http"
	"encoding/json"
)

func TestIsolatePlayersData(t *testing.T) {
	url := "http://www.funimation.com/shows/netoge/videos/official/and-you-thought-there-is-never-a-girl-online"
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != 200 {
		t.Fatal(fmt.Sprintf("playersData: got status code %d from %s", res.StatusCode, url))
	}

	jsonBytes, err := isolatePlayersDataJson(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	var dst interface{}
	if err := json.Unmarshal(jsonBytes, &dst); err != nil {
		t.Fatal(err)
	}

	t.Log(dst)
}

func TestGetPlayersData(t *testing.T) {
	jsonBytes := []byte("[{\"playerId\":\"showsPlayer\",\"userId\":true,\"solution\":\"flash\",\"playlist\":[{\"itemId\":\"5485\",\"itemAK\":\"Season 1\",\"itemType\":\"container\",\"itemClass\":\"season\",\"showId\":\"7556960\",\"showUrl\":\"netoge\",\"artist\":\"And you thought there is never a girl online?\",\"title\":\"Season 1\",\"description\":\"\",\"posterUrl\":null,\"items\":[{\"itemId\":\"33632\",\"itemAK\":\"and-you-thought-there-is-never-a-girl-online\",\"itemType\":\"clip\",\"itemClass\":\"recap\",\"showId\":\"7556960\",\"showUrl\":\"netoge\",\"videoType\":\"official\",\"videoUrl\":\"and-you-thought-there-is-never-a-girl-online\",\"artist\":\"And you thought there is never a girl online?\",\"title\":\"1 - And you thought there is never a girl online?\",\"description\":\"Hardcore otaku Hideki Nishimura enjoys playing a net game with other members of his online guild, and finally agrees to marry one within the game. However, when...\",\"posterUrl\":\"http://www.funimation.com/admin/uploads/default/recap_thumbnails/7556960/videos_spotlight/AYT0001.jpg\",\"favorited\":false,\"enqueued\":false,\"checkpoint\":0,\"videoSet\":[{\"videoId\":\"61269\",\"videoType\":\"official\",\"languageMode\":\"sub\",\"authToken\":\"?S9bgFtVlquI_pkPz3m-Hw3cvCnuOBf1AEfQjCSI36_jmY_NzXn5aLsnsyR-JSW5avaMj94iFAAQOTxnrPE_wF2bNh3VQSg28I1DZBQpAMnRNfMh3KWfjWN58rBcWGXsjSJBOqbTT2KIdPJmHMZ7m_g\",\"aspectRatio\":\"16:9\",\"duration\":1482,\"AIPs\":[],\"sdUrl\":\"http://wpc.8c48.edgecastcdn.net/038C48/SV/480/AYTJPNFSipon0001/AYTJPNFSipon0001-480-,750,1500,K.mp4.m3u8\",\"hdUrl\":\"nonSubscription\",\"hd1080Url\":\"nonSubscription\",\"huluId\":null,\"exclusive\":false,\"adSupported\":false,\"closedCaptions\":false,\"ccUrl\":null,\"videoNumber\":\"1.0\",\"title\":\"And you thought there is never a girl online?\",\"royalID\":\"SM-00000\",\"contractID\":\"1729\",\"videoAction\":\"Free Streaming\",\"FUNImationID\":\"AYTJPNFSipon0001\"}],\"number\":\"1.0\"},{\"itemType\":\"clip\",\"title\":\"2 - I thought we couldn't play net games at school?\",\"description\":\"Having met his other in-game guild members in real life, Hideki begins to picture them in their characters' places. Ako's behavior at school the next day makes ...\",\"videoUrl\":\"http://www.funimation.com/shows/netoge/videos/official/i-thought-we-couldnt-play-net-games-at-school\",\"number\":\"2.0\",\"posterUrl\":\"http://www.funimation.com/admin/uploads/default/recap_thumbnails/7556960/videos_spotlight/AYT0002.jpg\"}]}],\"selectedItemAK\":\"and-you-thought-there-is-never-a-girl-online\",\"selectedItemCheckpoint\":\"0\",\"size\":\"large\",\"mode\":\"full\",\"languageMode\":\"sub\",\"qualityMode\":\"sd\",\"autoPlay\":false,\"IDuser\":\"2629531\",\"userRole\":\"Past Subscriber\"}]")

	playersData, err := getPlayersData(jsonBytes)
	if err != nil {
		t.Fatal(err)
	}

	var logPlaylistItem func(pi playlistItem)
	logPlaylistItem = func(pi playlistItem) {
		if container, ok := pi.(*playlistItemContainer); ok {
			for _, item := range container.items {
				t.Log(item)
				logPlaylistItem(item)
			}
		} else if clip, ok := pi.(*playlistItemClip); ok {
			for _, item := range clip.videoSet {
				t.Log(item)
			}
		}
	}

	for _, pd := range playersData {
		t.Log(pd)
		for _, pi := range pd.playlist {
			t.Log(pi)
			logPlaylistItem(pi)
		}
	}
}

func TestGetPlayersDataFromUrl(t *testing.T) {

}