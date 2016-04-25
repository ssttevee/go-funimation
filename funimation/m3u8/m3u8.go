package m3u8

import (
	"net/http"
	"bufio"
	"errors"
	"strconv"
	"fmt"
	"strings"
	"io"
)

const extInfPrefix = "#EXTINF:"
const extInfLen = len(extInfPrefix)
const extEnd = "#EXT-X-ENDLIST"

type M3u8 struct {
	frags   []*fragment
	totalSize int64
}

func New(url string) (*M3u8, error) {
	baseUrl := url[:strings.LastIndex(url, "/") + 1]

	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	s := bufio.NewScanner(res.Body)
	s.Split(bufio.ScanLines)

	ret := &M3u8{
		frags: make([]*fragment, 0),
	}

	var currentFrag *fragment = nil
	for i := 0; s.Scan(); i++ {
		text := s.Text()

		if i == 0 && text != "#EXTM3U" {
			return nil, errors.New("Not m3u8 playlist")
		}

		if currentFrag != nil {
			if !strings.HasPrefix(text, "http") {
				text = baseUrl + text
			}
			currentFrag.url = text
			ret.frags = append(ret.frags, currentFrag)

			currentFrag = nil
		} else if len(text) > extInfLen && text[:extInfLen] == extInfPrefix {
			comma := strings.IndexRune(text, ',')

			duration, err := strconv.ParseFloat(text[extInfLen:comma], 64)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("Invalid fragment duration on line %d:%d", i + 1, extInfLen))
			}

			currentFrag = &fragment{
				seconds: duration,
			}
		} else if text == extEnd {
			// done!
			break
		}
	}

	res.Body.Close()

	return ret, nil
}

func (s *M3u8) Download(w io.Writer) (int64, error) {
	var totalSize int64
	for _, frag := range s.frags {
		size, err := frag.Save(w)
		if err != nil {
			return 0, err
		}

		totalSize += size
	}

	return totalSize, nil
}

func (s *M3u8) GetTotalSize() (int64, error) {
	sizeChan := make(chan int64)
	errChan := make(chan error)

	if s.totalSize > 0 {
		goto done
	}

	for _, frag := range s.frags {
		go func(f *fragment) {
			size, err := f.GetSize()
			if err != nil {
				errChan <- err
				return
			}

			sizeChan <- size
		}(frag)
	}

	for _ = range s.frags {
		select {
		case err := <- errChan:
			return 0, err
		case size := <- sizeChan:
			s.totalSize += size
		}
	}

	done:
	return s.totalSize, nil
}
