package source

import (
	"bytes"
	"io"
	"os"
)

// HeadLines returns complete lines from the first max bytes of r.
// Lines longer than max (huge tool outputs) are skipped — never loaded whole.
func HeadLines(r io.Reader, max int) [][]byte {
	buf := make([]byte, max)
	n, _ := io.ReadFull(r, buf)
	buf = buf[:n]
	eof := n < max
	lines := bytes.Split(buf, []byte{'\n'})
	if !eof && len(lines) > 0 {
		lines = lines[:len(lines)-1] // last one is cut off
	}
	return lines
}

// TailLines returns complete lines from the last max bytes of f.
func TailLines(f *os.File, size int64, max int) [][]byte {
	if size <= int64(max) {
		return nil // HeadLines already covered the whole file
	}
	buf := make([]byte, max)
	n, _ := f.ReadAt(buf, size-int64(max))
	lines := bytes.Split(buf[:n], []byte{'\n'})
	if len(lines) > 0 {
		lines = lines[1:] // first one is cut off
	}
	return lines
}
