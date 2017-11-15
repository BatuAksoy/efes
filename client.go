package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/cenkalti/log"
)

// Client is for reading and writing files on Efes.
type Client struct {
	config     ClientConfig
	log        log.Logger
	trackerURL *url.URL
	httpClient http.Client
}

// NewClient creates a new Client.
func NewClient(c *Config) (*Client, error) {
	u, err := url.Parse(c.Client.TrackerURL)
	if err != nil {
		return nil, err
	}
	logger := log.NewLogger("client")
	if c.Debug {
		logger.SetLevel(log.DEBUG)
	}
	return &Client{
		config:     c.Client,
		trackerURL: u,
		log:        logger,
	}, nil
}

func (c *Client) Read(key, path string) error {
	paths, err := c.getPaths(key)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return errors.New("no path returned from tracker")
	}
	resp, err := http.Get(paths[0])
	if err != nil {
		return err
	}
	err = checkResponseError(resp, http.StatusOK)
	if err != nil {
		return err
	}
	var f *os.File
	if path == "-" {
		f = os.Stdout
	} else {
		f, err = os.Create(path)
		if err != nil {
			return err
		}
	}
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (c *Client) getPaths(key string) ([]string, error) {
	form := url.Values{}
	form.Add("key", key)
	var response GetPaths
	err := c.request(http.MethodGet, "get-paths", form, http.StatusOK, &response)
	return response.Paths, err
}

func (c *Client) request(method, urlPath string, params url.Values, statusCode int, response interface{}) error {
	var reqBody io.Reader
	if method == http.MethodPost {
		reqBody = strings.NewReader(params.Encode())
	}
	newURL := *c.trackerURL
	newURL.Path = path.Join(c.trackerURL.Path, urlPath)
	if method == http.MethodGet {
		newURL.RawQuery = params.Encode()
	}
	req, err := http.NewRequest(method, newURL.String(), reqBody)
	if err != nil {
		return err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	c.log.Debugln("request method:", req.Method, "path:", req.URL.Path)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	err = checkResponseError(resp, statusCode)
	if err != nil {
		return err
	}
	if response == nil {
		return nil
	}
	err = json.NewDecoder(resp.Body).Decode(response)
	if err != nil {
		return err
	}
	c.log.Debugf("%s got response: %#v", req.URL.Path, response)
	return nil
}

func (c *Client) Write(key, path string) error {
	var f *os.File
	var size int64
	if path == "-" {
		f = os.Stdin
		size = -1
	} else {
		var err error
		f, err = os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close() // nolint: errcheck
		fi, err := f.Stat()
		if err != nil {
			return err
		}
		size = fi.Size()
	}
	path, fid, devid, err := c.createOpen(size)
	if err != nil {
		return err
	}
	n, err := c.write(path, size, f)
	if err != nil {
		return err
	}
	return c.createClose(key, n, fid, devid)
}

func (c *Client) createOpen(size int64) (path string, fid, devid int64, err error) {
	form := url.Values{}
	if size > -1 {
		form.Add("size", strconv.FormatInt(size, 10))
	}
	var response CreateOpen
	err = c.request(http.MethodPost, "create-open", form, http.StatusOK, &response)
	return response.Path, response.Fid, response.Devid, err
}

func (c *Client) write(path string, size int64, r io.Reader) (int64, error) {
	c.log.Debugln("client chunk size:", c.config.ChunkSize)
	rc := newReadCounter(r)
	for i := 0; ; i++ {
		c.log.Debugf("sending chunk #%d", i)
		chunkReader := io.LimitReader(rc, int64(c.config.ChunkSize))
		req, err := http.NewRequest(http.MethodPatch, path, chunkReader)
		if err != nil {
			return rc.Count(), err
		}
		offset := rc.Count()
		c.log.Debugln("sending file from offset:", offset)
		req.Header.Add("efes-file-offset", strconv.FormatInt(offset, 10))
		if size > -1 {
			req.Header.Add("efes-file-length", strconv.FormatInt(size, 10))
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return rc.Count(), err
		}
		err = checkResponseError(resp, http.StatusOK)
		if err != nil {
			return rc.Count(), err
		}
		if size > -1 && rc.Count() >= size {
			// EOF is reached. Do not make a new PATCH request with empty body.
			return rc.Count(), nil
		}
	}
}

func (c *Client) createClose(key string, size, fid, devid int64) error {
	form := url.Values{}
	form.Add("key", key)
	form.Add("size", strconv.FormatInt(size, 10))
	form.Add("fid", strconv.FormatInt(fid, 10))
	form.Add("devid", strconv.FormatInt(devid, 10))
	return c.request(http.MethodPost, "create-close", form, http.StatusOK, nil)
}

// Delete the key on Efes.
func (c *Client) Delete(key string) error {
	form := url.Values{}
	form.Add("key", key)
	return c.request(http.MethodPost, "delete", form, http.StatusOK, nil)
}

// Exist checks the existing of a key on Efes.
func (c *Client) Exist(key string) (bool, error) {
	paths, err := c.getPaths(key)
	if err != nil {
		return false, err
	}
	return len(paths) > 0, nil
}

func checkResponseError(resp *http.Response, statusCode int) error {
	if resp.StatusCode >= 500 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
	}
	if resp.StatusCode >= 400 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("client error (%d): %s", resp.StatusCode, string(body))
	}
	if resp.StatusCode != statusCode {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}
	return nil
}
