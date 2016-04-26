package downloader

import (
	"net/http"
	"strconv"
	"math"
)

const defaultFragmentSize = int64(1 << 22) // 4MiB

type Downloader struct {
	url             string
	size            int64

	fragmentable    bool
	FragmentSize    int64

	HttpClient      *http.Client

	OnBytesReceived func(int)
}

func (d *Downloader) Url() string {
	return d.url
}

func (d *Downloader) Size() int64 {
	return d.size
}

func (d *Downloader) Fragmentable() bool {
	return d.fragmentable
}

func (d *Downloader) NumFragments() int {
	return int(math.Ceil(float64(d.size) / float64(d.FragmentSize)))
}

func New(url string) (*Downloader, error) {
	res, err := http.Head(url)
	if err != nil {
		return nil, err
	}

	length, err := strconv.ParseInt(res.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		return nil, err
	}

	fragmentable := res.Header.Get("Accept-Ranges") == "bytes"

	return &Downloader{
		url: url,
		size: length,
		fragmentable: fragmentable,
		FragmentSize: defaultFragmentSize,
	}, nil
}
