package library

import (
	"io"
	"os"
)

// io16k limits reading to the head of the file.
func io16k(f *os.File) io.Reader { return io.LimitReader(f, 16<<10) }
