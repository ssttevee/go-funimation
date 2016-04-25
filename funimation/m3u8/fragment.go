package m3u8

import (
	"net/http"
	"strconv"
	"io"
)

type fragment struct {
	seconds float64
	url     string
}

func (f *fragment) GetSize() (int64, error) {
	res, err := http.Head(f.url)
	if err != nil {
		return 0, err
	}

	length, err := strconv.ParseInt(res.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		return 0, err
	}

	return length, nil
}

func (f *fragment) Save(w io.Writer) (int64, error) {
	res, err := http.Get(f.url)
	if err != nil {
		return 0, err
	}

	count, err := io.Copy(w, res.Body)
	if err != nil {
		return 0, err
	}

	return count, nil
}
