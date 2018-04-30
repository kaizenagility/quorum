package constellation

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/tv42/httpunix"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func launchNode(cfgPath string) (*exec.Cmd, error) {
	cmd := exec.Command("constellation-node", cfgPath)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	go io.Copy(os.Stderr, stderr)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	time.Sleep(100 * time.Millisecond)
	return cmd, nil
}

func unixTransport(socketPath string) *httpunix.Transport {
	t := &httpunix.Transport{
		DialTimeout:           1 * time.Second,
		RequestTimeout:        5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	}
	t.RegisterLocation("c", socketPath)
	return t
}

func unixClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: unixTransport(socketPath),
	}
}

func httpTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}
}

func httpClient() *http.Client {
	return &http.Client{
		Timeout:   time.Second * 5,
		Transport: httpTransport(),
	}
}

func UpCheck(c *Client) error {
	res, err := c.httpClient.Get(c.BaseURL + "upcheck")
	if err != nil {
		return err
	}
	if res.StatusCode == 200 {
		return nil
	}
	return errors.New("Constellation Node API did not respond to upcheck request")
}

type Client struct {
	httpClient *http.Client
	BaseURL    string
}

func (c *Client) SendPayload(pl []byte, b64From string, b64To []string) ([]byte, error) {
	buf := bytes.NewBuffer(pl)
	req, err := http.NewRequest("POST", c.BaseURL+"sendraw", buf)
	if err != nil {
		return nil, err
	}
	if b64From != "" {
		req.Header.Set("c11n-from", b64From)
	}
	req.Header.Set("c11n-to", strings.Join(b64To, ","))
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		io.Copy(ioutil.Discard, res.Body)
		return nil, fmt.Errorf("Non-200 status code: %+v", res)
	}
	body, err := ioutil.ReadAll(base64.NewDecoder(base64.StdEncoding, res.Body))
	io.Copy(ioutil.Discard, res.Body)
	return body, err
}

func (c *Client) ReceivePayload(key []byte) ([]byte, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"receiveraw", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("c11n-key", base64.StdEncoding.EncodeToString(key))
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		io.Copy(ioutil.Discard, res.Body)
		return nil, fmt.Errorf("Non-200 status code: %+v", res)
	}
	body, err := ioutil.ReadAll(res.Body)
	io.Copy(ioutil.Discard, res.Body)
	return body, err
}

func NewClient(config *Config) (*Client, error) {
	var client *http.Client
	var baseURL string
	if config.Socket != "" {
		socketPath := filepath.Join(config.WorkDir, config.Socket)
		client = unixClient(socketPath)
		baseURL = "http+unix://c/"
	} else {
		client = httpClient()
		baseURL = config.BaseURL
		if baseURL[len(baseURL)-1:] != "/" {
			baseURL += "/"
		}
	}
	return &Client{
		httpClient: client,
		BaseURL:    baseURL,
	}, nil
}
