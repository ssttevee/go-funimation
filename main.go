package main

import (
	"fmt"
	"github.com/ssttevee/funimation/lib"
	"net/http/cookiejar"
	"log"
	"io"
	"os"
	"time"
	"github.com/dustin/go-humanize"
	"flag"
	"strings"
	"strconv"
	"github.com/ssttevee/go-downloader"
)

type writerMiddleware struct {
	Writer io.Writer
	Func func(int)
}

func (w *writerMiddleware) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.Func(n)
	return n, err
}

var funimationClient *funimation.Client

func init() {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatal(err.Error())
		return
	}

	funimationClient = funimation.New(jar)
}

func main() {
	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	listCmd.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: funimation list <show>")
		fmt.Fprintln(os.Stderr, "    OR funimation list <show-url>\n")
		fmt.Fprintln(os.Stderr, "Lists all episodes in the given show\n\n")
	}

	downloadCmd := flag.NewFlagSet("download", flag.ExitOnError)
	downloadCmd.String("email", "", "your funimation account email address")
	downloadCmd.String("password", "", "your funimation account password")
	downloadCmd.Int("bitrate", 0, "will take the closest `Kbps` bitrate if the given is not available, 0 = best")
	downloadCmd.String("language", funimation.Subbed, "either `sub or dub`")
	downloadCmd.Bool("url-only", false, "get the url instead of downloading")
	downloadCmd.Int("threads", 1, "number of threads for multithreaded download")
	downloadCmd.Bool("enable-url-guessing", false, "guess urls for non-public videos")
	downloadCmd.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: funimation download [options] <show> <episode> [<episode>...]")
		fmt.Fprintln(os.Stderr, "    OR funimation download [options] <show> <episode-nums> [<episode-nums>...]")
		fmt.Fprintln(os.Stderr, "    OR funimation download [options] <show-id> <episode-nums> [<episode-nums>...]")
		fmt.Fprintln(os.Stderr, "    OR funimation download [options] <episode-url> [<episode-url>...]\n")
		fmt.Fprintln(os.Stderr, "Downloads an episode from the given show\n")
		fmt.Fprintln(os.Stderr, "Options:")
		downloadCmd.PrintDefaults()
	}

	if len(os.Args) == 1 {
		fmt.Println("Usage: funimation <command> [<args>]\n")
		fmt.Println("Available commands are: ")
		fmt.Println("  list      Lists all episodes in the given series")
		fmt.Println("  download  Downloads an episode from the given series")
		return
	}

	switch os.Args[1] {
	case "list":
		listCmd.Parse(os.Args[2:])
		break
	case "download":
		downloadCmd.Parse(os.Args[2:])
		break
	default:
		fmt.Printf("%q is not valid command.\n", os.Args[1])
		os.Exit(2)
	}

	switch {
	case listCmd.Parsed():
		show := listCmd.Arg(0)
		if show == "" {
			listCmd.Usage()
			os.Exit(2)
		}
		doList(show)
	case downloadCmd.Parsed():
		doDownload(downloadCmd)
	}
}

func doList(show string) {
	series, err := funimationClient.GetSeries(show)
	if err != nil {
		log.Fatal("Failed to get series: ", err)
	}

	episodes, err := series.GetAllEpisodes()
	if err != nil {
		log.Fatal("Failed to get episodes: ", err)
	}

	fmt.Println(series.Title())
	fmt.Println(series.Description())
	fmt.Println()
	fmt.Print(episodes.String())
}

func doDownload(cmd *flag.FlagSet) {
	show := cmd.Arg(0)
	if show == "" || !strings.HasPrefix(show, "http") && cmd.Arg(1) == "" {
		cmd.Usage()
		os.Exit(2)
	}

	if email := cmd.Lookup("email").Value.(flag.Getter).Get().(string); email != "" {
		password := cmd.Lookup("password").Value.(flag.Getter).Get().(string)

		if password == "" {
			log.Fatal("Got `email` flag but missing `password` flag")
		}

		if err := funimationClient.Login(email, password); err != nil {
			log.Fatal("Login failed: ", err)
		}
	}

	episodes := make([]*funimation.Episode, 0)

	funimation.GuessUrls = cmd.Lookup("enable-url-guessing").Value.(flag.Getter).Get().(bool)

	if strings.HasPrefix(show, "http") {
		for _, url := range cmd.Args() {
			if !strings.HasPrefix(url[strings.Index(url, "://"):], "://www.funimation.com/shows/") {
				log.Println("Only funimation show urls are allowed")
				continue
			}

			episode, err := funimationClient.GetEpisodeFromUrl(url)
			if err != nil {
				log.Println("Failed to get episode: ", err)
				continue
			}

			episodes = append(episodes, episode)
		}
	} else {
		var series *funimation.Series
		if showNum, err := strconv.ParseInt(show, 10, 32); err != nil {
			// not a show number, assume it is a show slug
			series, err = funimationClient.GetSeries(show)
			if err != nil {
				log.Fatal("Failed to get series: ", err)
			}
		} else {
			series, err = funimationClient.GetSeriesById(int(showNum))
			if err != nil {
				log.Fatal("Failed to get series: ", err)
			}
		}

		for i := 1; i < cmd.NArg(); i++ {
			arg := cmd.Arg(i)
			if arg == "*" {
				if eps, err := series.GetAllEpisodes(); err != nil {
					log.Println("Failed to get all episodes: ", err)
					continue
				} else {
					episodes = eps
					break
				}
			} else if strings.ContainsRune(arg, '-') {
				startEnd := strings.Split(arg, "-")
				if len(startEnd) != 2 {
					log.Printf("Range value `%s` must contain 1 dash character\n", arg)
					continue
				}

				start, err := strconv.ParseInt(startEnd[0], 10, 32)
				if err != nil {
					log.Println("Range value must be numeric")
					continue
				}

				end, err := strconv.ParseInt(startEnd[1], 10, 32)
				if err != nil {
					log.Println("Range value must be numeric")
					continue
				}

				eps, err := series.GetEpisodesRange(int(start), int(end))
				if err != nil {
					log.Println(err)
					continue
				}

				episodes = append(episodes, eps...)
			} else if epNum, err := strconv.ParseInt(arg, 10, 32); err != nil {
				episode, err := series.GetEpisodeBySlug(arg)
				if err != nil {
					log.Println("Failed to get episode: ", err)
					continue
				}

				episodes = append(episodes, episode)
			} else {
				episode, err := series.GetEpisode(int(epNum))
				if err != nil {
					log.Println("Failed to get episode: ", err)
					continue
				}

				episodes = append(episodes, episode)
			}
		}
	}

	if len(episodes) == 0 {
		cmd.Usage()
		os.Exit(2)
	}

	fmt.Printf("Found %d episodes:\n", len(episodes))

	bitrate := cmd.Lookup("bitrate").Value.(flag.Getter).Get().(int)
	language := funimation.EpisodeLanguage(cmd.Lookup("language").Value.(flag.Getter).Get().(string))
	urlOnly := cmd.Lookup("url-only").Value.(flag.Getter).Get().(bool)
	threads := cmd.Lookup("threads").Value.(flag.Getter).Get().(int)

	// default to subbed
	if language != funimation.Subbed && language != funimation.Dubbed {
		fmt.Println("Received unknown language mode; defaulting to sub")
		language = funimation.Subbed
	}

	for _, episode := range episodes {
		foundLang := false
		hasSub := false
		for _, l := range episode.Languages() {
			if l == funimation.Subbed {
				hasSub = true
			}
			if language == l {
				foundLang = true
			}
		}

		// make sure language is available
		if !foundLang {
			fmt.Printf("Language mode - %s - is unavailable; ", language)

			// if subbed is unavailable, something is wrong... just give up...
			if !hasSub {
				log.Println("skipping...")
				continue
			}

			fmt.Println("defaulting to sub")
			language = funimation.Subbed
		}

		if bitrate == 0 {
			bitrate = episode.FixBitrate(bitrate, language)
			if bitrate == 0 {
				log.Println("Can't download that episode")
				continue
			}
		}

		assertBitrate(episode, bitrate, language)

		if urlOnly {
			url, err := episode.GetMp4Url(bitrate, language)
			if err != nil {
				log.Println("Failed to get url: ", err)
				continue
			}

			fmt.Printf("Season %d, Episode %v: %s\n", episode.SeasonNumber(), episode.EpisodeNumber(), url)
			continue
		}

		dl, err := episode.GetDownloader(bitrate, language)
		if err != nil {
			log.Println("Failed to get downloader: ", err)
			continue
		}

		fname := fmt.Sprintf("s%de%v - %s [%s][%s].mp4", episode.SeasonNumber(), episode.EpisodeNumber(), episode.Title(), bitrateToQuality(bitrate), language)
		fname = strings.Map(func(r rune) (rune) {
			if r == '\\' || r == '/' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
				return -1
			}
			return r
		}, fname)

		fmt.Printf("\nDownloading %s Season %d - Episode %v\n", episode.Title(), episode.SeasonNumber(), episode.EpisodeNumber())
		fmt.Printf("Saving to: %s\n\n", fname)

		startTime := time.Now()

		bytesStrLen := len(humanize.Comma(dl.Size()))

		var lastTime = time.Now()
		var rate float64
		var bytesSinceLastTime int64
		var d *downloader.Download
		dl.OnBytesReceived = func(bytes int) {
			bytesSinceLastTime += int64(bytes)

			percent := d.Percent()
			newTime := time.Now()
			timeDiff := newTime.Sub(lastTime)
			if secs := timeDiff.Seconds(); secs >= 0.5 {
				rate = float64(bytesSinceLastTime) / secs
				lastTime = newTime
				bytesSinceLastTime = 0
			}

			percentStr := fmt.Sprintf("%.2f", percent * float32(100))
			for ; len(percentStr) < 6; {
				percentStr = " " + percentStr
			}

			progBar := "["
			progBarLen := 30
			for i := 0; i < progBarLen; i++ {
				if float32(i) < float32(progBarLen) * percent {
					if progBar[len(progBar) - 1] == byte('>') {
						progBar = progBar[:len(progBar) - 1] + "="
					}
					progBar += ">"
				} else {
					progBar += " "
				}
			}
			progBar += "]"

			bytesStr := humanize.Comma(int64(d.Current()))
			for ; len(bytesStr) < bytesStrLen; {
				bytesStr = " " + bytesStr
			}

			rateStr := humanize.Bytes(uint64(rate))
			for ; len(rateStr) < 10; {
				rateStr = " " + rateStr
			}

			fmt.Printf("\r%s%% %s %s %s/s", percentStr, progBar, bytesStr, rateStr)
		}

		fmt.Println()
		if d, err = dl.Download(fname, threads); err != nil {
			log.Println("Failed to start download: ", err)
		} else {
			if err := d.Wait(); err != nil {
				log.Println("\nDownload failed: ", err)
				continue
			}

			fmt.Printf("\nDownloaded %s in %v\n", humanize.Bytes(uint64(dl.Size())), time.Now().Sub(startTime))
		}
	}
}

func assertBitrate(ep *funimation.Episode, bitrate int, language funimation.EpisodeLanguage) {
	for _, br := range ep.BitRates(language) {
		if br == bitrate {
			return
		}
	}

	log.Fatal("That bitrate is unavailable")
}

func bitrateToQuality(bitRate int) (string) {
	if bitRate <= 1500 {
		return "SD"
	} else if bitRate <= 2500 {
		return "720p"
	} else if bitRate <= 4000 {
		return "1080p"
	} else {
		return ""
	}
}
