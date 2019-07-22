package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/cenkalti/log"
)

// Client is for reading and writing files on Efes.
type Client struct {
	config     *Config
	log        log.Logger
	trackerURL *url.URL
	httpClient http.Client
}

// NewClient creates a new Client.
func NewClient(cfg *Config) (*Client, error) {
	u, err := url.Parse(cfg.Client.TrackerURL)
	if err != nil {
		return nil, err
	}
	c := &Client{
		config:     cfg,
		trackerURL: u,
		log:        log.NewLogger("client"),
	}
	c.httpClient.Timeout = time.Duration(cfg.Client.SendTimeout)
	if cfg.Debug {
		c.log.SetLevel(log.DEBUG)
	}
	return c, nil
}

func (c *Client) request(method, urlPath string, params url.Values, response interface{}) (*http.Response, error) {
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
		return nil, err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	c.log.Debugln("request method:", req.Method, "path:", req.URL.Path, "params:", params)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	err = checkResponseError(resp)
	if err != nil {
		return resp, err
	}
	if response == nil {
		return resp, nil
	}
	err = json.NewDecoder(resp.Body).Decode(response)
	if err != nil {
		return resp, err
	}
	c.log.Debugf("%s got response: %#v", req.URL.Path, response)
	return resp, nil
}

// Delete the key on Efes.
func (c *Client) Delete(key string) error {
	form := url.Values{}
	form.Add("key", key)
	_, err := c.request(http.MethodPost, "delete", form, nil)
	return err
}

// Exists checks the existence of a key on Efes.
func (c *Client) Exists(key string) (bool, error) {
	_, err := c.getPath(key)
	if err != nil {
		if errc, ok := err.(*ClientError); ok && errc.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
