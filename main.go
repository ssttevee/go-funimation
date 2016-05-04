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
	downloader.TempDir = os.TempDir() + "/.funimation"

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
	downloadCmd.String("quality", "max", "quality of the video, `sd, hd, or fhd`")
	downloadCmd.String("language", funimation.Subbed, "either `sub or dub`")
	downloadCmd.Bool("url-only", false, "get the url instead of downloading")
	downloadCmd.Int("threads", 1, "number of threads for multithreaded download")
	downloadCmd.Bool("guess", false, "guess urls for non-public videos")
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
		log.Fatal(err)
	}

	episodes, err := series.GetAllEpisodes()
	if err != nil {
		log.Fatal(err)
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
			log.Fatal("login: ", err)
		}
	}

	episodes := make([]*funimation.Episode, 0)
	getEpisode := func(f func() (*funimation.Episode, error)) {
		episode, err := f()
		if err != nil {
			log.Fatal(err)
		}

		episodes = append(episodes, episode)
	}

	if strings.HasPrefix(show, "http") {
		for _, url := range cmd.Args() {
			if !strings.HasPrefix(url[strings.Index(url, "://"):], "://www.funimation.com/shows/") {
				log.Fatal("Only funimation show urls are allowed")
			}

			getEpisode(func() (*funimation.Episode, error) {
				return funimationClient.GetEpisodeFromUrl(url)
			})
		}
	} else {
		var series *funimation.Series
		if showNum, err := strconv.ParseInt(show, 10, 32); err != nil {
			// not a show number, assume it is a show slug
			series, err = funimationClient.GetSeries(show)
			if err != nil {
				log.Fatal(err)
			}
		} else {
			series, err = funimationClient.GetSeriesById(int(showNum))
			if err != nil {
				log.Fatal(err)
			}
		}

		for i := 1; i < cmd.NArg(); i++ {
			arg := cmd.Arg(i)
			if arg == "*" {
				if eps, err := series.GetAllEpisodes(); err != nil {
					log.Fatal(err)
				} else {
					episodes = eps
					break
				}
			} else if strings.ContainsRune(arg, '-') {
				startEnd := strings.Split(arg, "-")
				if len(startEnd) != 2 {
					log.Fatalf("Range value `%s` must contain 1 dash character", arg)
				}

				start, err := strconv.ParseInt(startEnd[0], 10, 32)
				if err != nil {
					log.Fatal("Range value must be numeric")
				}

				end, err := strconv.ParseInt(startEnd[1], 10, 32)
				if err != nil {
					log.Fatal("Range value must be numeric")
				}

				eps, err := series.GetEpisodesRange(int(start), int(end))
				if err != nil {
					log.Fatal(err)
				}

				episodes = append(episodes, eps...)
			} else if epNum, err := strconv.ParseInt(arg, 10, 32); err != nil {
				getEpisode(func() (*funimation.Episode, error) {
					return series.GetEpisodeBySlug(arg)
				})
			} else {
				getEpisode(func() (*funimation.Episode, error) {
					return series.GetEpisode(int(epNum))
				})
			}
		}
	}

	if len(episodes) == 0 {
		cmd.Usage()
		os.Exit(2)
	}

	fmt.Printf("Found %d episodes:\n", len(episodes))

	quality := cmd.Lookup("quality").Value.(flag.Getter).Get().(string)
	language := funimation.EpisodeLanguage(cmd.Lookup("language").Value.(flag.Getter).Get().(string))
	urlOnly := cmd.Lookup("url-only").Value.(flag.Getter).Get().(bool)
	threads := cmd.Lookup("threads").Value.(flag.Getter).Get().(int)
	guessUrls := cmd.Lookup("guess").Value.(flag.Getter).Get().(bool)

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
		var el funimation.EpisodeLanguage = language
		if !foundLang {
			fmt.Printf("Language mode - %s - is unavailable; ", el)

			// if subbed is unavailable, something is wrong... just give up...
			if !hasSub {
				log.Fatal("exiting...")
				os.Exit(2)
			}

			fmt.Println("defaulting to sub")
			el = funimation.Subbed
		}

		var eq funimation.EpisodeQuality
		if quality == "max" {
			eq = episode.GetBestQuality(el, !guessUrls)
		} else {
			eq = funimation.ParseEpisodeQuality(quality)
		}

		urlFunc := episode.GetVideoUrl
		if guessUrls {
			urlFunc = episode.GuessVideoUrl
		}

		url, err := urlFunc(el, eq)
		if err != nil {
			log.Println("Failed to get url: ", err)
			continue
		}

		if urlOnly {
			fmt.Printf("Season %d, Episode %v: %s\n", episode.SeasonNumber(), episode.EpisodeNumber(), url)
			continue
		}

		dl, err := downloader.New(url)
		if err != nil {
			log.Fatal("get stream: ", err)
		}

		fname := fmt.Sprintf("s%de%v - %s [%s][%s].mp4", episode.SeasonNumber(), episode.EpisodeNumber(), episode.Title(), eq.String(), el)
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
			log.Fatal("download: ", err)
		} else {
			if err := d.Wait(); err != nil {
				log.Fatal("wait: ", err)
			}

			fmt.Printf("\nDownloaded %s in %v\n", humanize.Bytes(uint64(dl.Size())), time.Now().Sub(startTime))
		}
	}
}
