package util

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
)

// Download - performs HTTP GET request against url, including adding gzip header and ungzipping
func Download(url string) (b []byte, err error) {
	var (
		req    *http.Request
		reader io.ReadCloser
	)

	// forming a request
	req, err = http.NewRequest("GET", url, nil)
	if err != nil {
		return b, err
	}
	req.Header.Add("Accept-Encoding", "gzip")

	// running it into a client
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return b, err
	}
	defer resp.Body.Close()

	// optionally decompressing it
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	return ioutil.ReadAll(reader)
}
