package downloader

import (
	"os"
	"fmt"
	"net/http"
	"io"
)

var tempDir = os.TempDir() + "/.funimation-ripper"

type byteCounterWriter struct {
	writer         *os.File
	onBytesWritten func(int)
}

func (w *byteCounterWriter) Write(p []byte) (n int, err error) {
	w.onBytesWritten(len(p))
	return (*w.writer).Write(p)
}

type dlJob struct {
	index     int
	byteStart int64
	byteEnd   int64
}

func (d *Downloader) Download(destFileName string, threads int) (int64, error) {
	// make sure there is at least one thread
	if threads < 1 {
		threads = 1
	}

	// make sure the temp dir exists
	os.Mkdir(tempDir, os.ModeDir)

	fragCount := d.NumFragments()
	fragmentFiles := make([]*os.File, fragCount)
	jobsChan := make(chan dlJob, fragCount)
	errsChan := make(chan error)

	// setup workers
	for i := 0; i < threads; i++ {
		go func() {
			for job := range jobsChan {
				fpath := fmt.Sprintf("%s/%s.mp4.part%d", tempDir, destFileName, job.index)

				if finfo, err := os.Stat(fpath); err == nil {
					if finfo.Size() == job.byteEnd - job.byteStart + 1 {
						// assume same size is synonymous with same file
						f, err := os.Open(fpath)
						if err != nil {
							errsChan <- err
							return
						}

						d.OnBytesReceived(int(d.FragmentSize))

						fragmentFiles[job.index] = f
						errsChan <- nil
						continue
					}
				}

				f, err := os.Create(fpath)
				if err != nil {
					errsChan <- err
					return
				}

				client := d.HttpClient
				if client == nil {
					client = http.DefaultClient
				}

				req, err := http.NewRequest("GET", d.url, nil)
				if err != nil {
					errsChan <- err
					return
				}
				req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", job.byteStart, job.byteEnd))

				res, err := client.Do(req)
				if err != nil {
					errsChan <- err
					return
				}

				if _, err := io.Copy(&byteCounterWriter{
					writer: f,
					onBytesWritten: d.OnBytesReceived,
				}, res.Body); err != nil {
					errsChan <- err
					return
				}

				fragmentFiles[job.index] = f
				errsChan <- nil
			}
		}()
	}

	for i := 0; i < fragCount; i++ {
		job := dlJob{index: i}

		job.byteStart = int64(i) * d.FragmentSize
		if i == fragCount - 1 {
			job.byteEnd = d.size - 1
		} else {
			job.byteEnd = int64(i + 1) * d.FragmentSize - 1
		}

		jobsChan <- job
	}

	for range fragmentFiles {
		if err := <-errsChan; err != nil {
			return 0, err
		}
	}

	out, err := os.Create(fmt.Sprintf("%s.mp4", destFileName))
	if err != nil {
		return 0, err
	}

	totalBytesCopied := int64(0)
	for _, f := range fragmentFiles {
		if copied, err := io.Copy(out, f); err != nil {
			return 0, err
		} else {
			totalBytesCopied += copied
		}

		f.Close()
		os.Remove(f.Name())
	}

	return totalBytesCopied, nil
}
