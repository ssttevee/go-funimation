package m3u8

import (
	"os"
	"fmt"
	"io"
)

var tempDir = os.TempDir() + "/.funimation-ripper"

type downloadJob struct {
	fname          string
	index          int
	onBytesWritten func(int)
	frag           *fragment
}

type downloadResult struct {
	err   error
	file  *os.File
	index int
}

type byteCounterWriter struct {
	writer         *os.File
	onBytesWritten func(int)
}

func (w *byteCounterWriter) Write(p []byte) (n int, err error) {
	w.onBytesWritten(len(p))
	return (*w.writer).Write(p)
}

func (s *M3u8) Download(filename string, threads int, onBytesWritten func(int)) (int64, error) {
	if threads < 1 {
		threads = 1
	}

	jobChan := make(chan downloadJob, len(s.frags))
	resChan := make(chan downloadResult, 10)

	for i, frag := range s.frags{
		jobChan <- downloadJob{
			fname: filename,
			index: i,
			onBytesWritten: onBytesWritten,
			frag: frag,
		}
	}
	close(jobChan)

	os.Mkdir(tempDir, os.ModeDir)
	for i := 0; i < threads; i++ {
		go downloadWorker(jobChan, resChan)
	}

	fragFiles := make([]*os.File, len(s.frags))
	for _ = range s.frags {
		res := <-resChan
		if res.err != nil {
			return 0, res.err
		}

		fragFiles[res.index] = res.file
	}

	out, err := os.Create(fmt.Sprintf("%s.mp4.ts", filename))
	if err != nil {
		return 0, err
	}

	var totalSize int64
	for _, f := range fragFiles {
		io.Copy(out, f)
		f.Close()
		os.RemoveAll(f.Name())
	}

	return totalSize, nil
}

func downloadWorker(in <-chan downloadJob, out chan<- downloadResult) {
	for job := range in {
		fpath := fmt.Sprintf("%s/%s.mp4.ts.part%d", tempDir, job.fname, job.index)

		if finfo, err := os.Stat(fpath); err == nil {
			size, err := job.frag.GetSize()
			if err != nil {
				out <- downloadResult{err: err}
				return
			}

			if finfo.Size() == size {
				// assume same size is synonymous with same file
				f, err := os.Open(fpath)
				if err != nil {
					out <- downloadResult{err: err}
					return
				}

				job.onBytesWritten(int(size))
				out <- downloadResult{file: f, index: job.index}
				continue
			}
		}

		f, err := os.Create(fpath)
		if err != nil {
			out <- downloadResult{err: err}
		}

		_, err = job.frag.Save(&byteCounterWriter{
			writer: f,
			onBytesWritten: job.onBytesWritten,
		})
		out <- downloadResult{file: f, index: job.index}
	}

}