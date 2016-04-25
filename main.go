package main

import (
	"fmt"
	"ssttevee.com/funimation-downloader/funimation"
	"net/http/cookiejar"
	"log"
	"io"
	"os"
	"time"
	"github.com/dustin/go-humanize"
	"flag"
	"strings"
	"strconv"
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
	downloadCmd.String("out", "", "`file` save location")
	downloadCmd.Bool("mock", false, "do everything normally but don't download the video")
	downloadCmd.Int("threads", 1, "number of threads for multithreaded download")
	downloadCmd.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: funimation download [options] <show> <episode>")
		fmt.Fprintln(os.Stderr, "    OR funimation download [options] <show> <episode-num>")
		fmt.Fprintln(os.Stderr, "    OR funimation download [options] <show-id> <episode-num>")
		fmt.Fprintln(os.Stderr, "    OR funimation download [options] <episode-url>\n")
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
	episode := cmd.Arg(1)
	if show == "" || !strings.HasPrefix(show, "http") && episode == "" {
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

	var ep *funimation.Episode = nil
	if strings.HasPrefix(show, "http") {
		if !strings.HasPrefix(show[strings.Index(show, "://"):], "://www.funimation.com/shows/") {
			log.Fatal("Only funimation show urls are allowed")
		}

		var err error
		ep, err = funimationClient.GetEpisodeFromUrl(show)
		if err != nil {
			log.Fatal("get episode from url: ", err)
		}
	} else {
		showNum, err := strconv.ParseInt(show, 10, 32)
		if err != nil {
			// not a show number, assume it is a show slug
			epNum, err := strconv.ParseInt(episode, 10, 32)
			if err != nil {
				ep, err = funimationClient.GetEpisodeFromShowEpisodeSlug(show, episode)
			} else {
				s, err := funimationClient.GetSeries(show)
				if err != nil {
					log.Fatal("get series: ", err)
				}

				ep, err = s.GetEpisode(int(epNum))
				if err != nil {
					log.Fatal("get episode: ", err)
				}
			}
		} else {
			epNum, err := strconv.ParseInt(episode, 10, 32)
			if err != nil {
				log.Fatal("Episode number must be a number")
			}

			ep, err = funimationClient.GetEpisode(int(showNum), int(epNum))
		}
	}

	if ep == nil {
		cmd.Usage()
		os.Exit(2)
	}

	bitrate := cmd.Lookup("bitrate").Value.(flag.Getter).Get().(int)
	language := funimation.EpisodeLanguage(cmd.Lookup("language").Value.(flag.Getter).Get().(string))
	mock := cmd.Lookup("mock").Value.(flag.Getter).Get().(bool)
	fname := cmd.Lookup("out").Value.(flag.Getter).Get().(string)
	threads := cmd.Lookup("threads").Value.(flag.Getter).Get().(int)

	// default to subbed
	if language != funimation.Subbed && language != funimation.Dubbed {
		fmt.Println("Received unknown language mode; defaulting to sub")
		language = funimation.Subbed
	}

	foundLang := false
	hasSub := false
	for _, l := range ep.Languages() {
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
			log.Fatal("exiting...")
			os.Exit(2)
		}

		fmt.Println("defaulting to sub")
		language = funimation.Subbed
	}

	if bitrate == 0 {
		bitrate = ep.FixBitrate(bitrate, language)
	}

	if bitrate == 0 {
		log.Fatal("Can't download that episode")
	}

	assertBitrate(ep, bitrate, language)

	stream, err := ep.GetStream(bitrate, language)
	if err != nil {
		log.Fatal("get stream: ", err)
	}

	videoSize, err := stream.GetTotalSize()
	if err != nil {
		log.Fatal("get total size: ", err)
	}

	if fname == "" {
		fname = fmt.Sprintf("s%de%v - %s [%s][%s]", ep.SeasonNumber(), ep.EpisodeNumber(), ep.Title(), bitrateToQuality(bitrate), language)
	}

	// remove characters that are not allowed in filenames
	fname = strings.Map(func(r rune) (rune) {
		if r == '\\' || r == '/' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return -1
		}
		return r
	}, fname)

	fmt.Printf("Downloading %s Season - %d Episode %v\n", ep.Title(), ep.SeasonNumber(), ep.EpisodeNumber())
	fmt.Printf("Saving to: %s\n\n", fname)

	progress := int64(0)
	startTime := time.Now()
	if mock {
		fmt.Println("Received mock flag, not downloading...")
	} else {
		bytesStrLen := len(humanize.Comma(videoSize))

		lastTime := time.Now()
		var rate float64 = 0
		var bytesSinceLastTime int64 = 0
		onBytesWritten := func(bytes int) {
			progress += int64(bytes)
			bytesSinceLastTime += int64(bytes)

			percent := float32(progress) / float32(videoSize)
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

			bytesStr := humanize.Comma(progress)
			for ; len(bytesStr) < bytesStrLen; {
				bytesStr = " " + bytesStr
			}

			rateStr := humanize.Bytes(uint64(rate))
			for ; len(rateStr) < 10; {
				rateStr = " " + rateStr
			}

			fmt.Printf("\r%s%% %s %s %s/s", percentStr, progBar, bytesStr, rateStr)
		}

		if _, err := stream.Download(fname, threads, onBytesWritten); err != nil {
			fmt.Println()
			log.Fatal("download: ", err)
		}
		fmt.Println()
	}
	endTime := time.Now()

	fmt.Printf("\nDownloaded %s in %v\n", humanize.Bytes(uint64(progress)), endTime.Sub(startTime))
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
