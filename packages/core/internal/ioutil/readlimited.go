// Package ioutil provides internal I/O utilities.
package ioutil

import "io"

// ReadLimited reads up to limit bytes from r.
// If the stream produces exactly limit bytes the caller should treat it as
// truncated (same convention as the Rust implementation).
func ReadLimited(r io.Reader, limit int) ([]byte, error) {
	if limit <= 0 {
		return []byte{}, nil
	}
	initCap := limit
	if initCap > 64*1024 {
		initCap = 64 * 1024
	}
	buf := make([]byte, 0, initCap)
	tmp := make([]byte, 8192)
	total := 0

	for total < limit {
		remaining := limit - total
		toRead := len(tmp)
		if remaining < toRead {
			toRead = remaining
		}
		n, err := r.Read(tmp[:toRead])
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			total += n
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return buf, err //nolint:wrapcheck // error from io.Reader interface; wrapping adds no useful context here
		}
	}
	return buf, nil
}

