package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"haven/internal/object"
)

// Client talks to a haven host at BaseURL.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient builds a client for a base URL (trailing slash trimmed).
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    http.DefaultClient,
	}
}

// Info fetches repository metadata.
func (c *Client) Info() (Info, error) {
	var info Info
	err := c.getJSON("/info", &info)
	return info, err
}

// Refs fetches the remote ref listing.
func (c *Client) Refs() ([]RefInfo, error) {
	var refs []RefInfo
	err := c.getJSON("/refs", &refs)
	return refs, err
}

// GetObject downloads one object.
func (c *Client) GetObject(hash string) (object.Type, []byte, error) {
	resp, err := c.HTTP.Get(c.BaseURL + "/objects/" + hash)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("get object %s: %s", hash, resp.Status)
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	return object.Type(resp.Header.Get(HeaderType)), content, nil
}

// PutObject uploads one object.
func (c *Client) PutObject(hash string, typ object.Type, content []byte) error {
	req, err := http.NewRequest(http.MethodPut, c.BaseURL+"/objects/"+hash, bytes.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set(HeaderType, string(typ))
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("put object %s: %s", hash, statusBody(resp))
	}
	return nil
}

// UpdateRef performs a conditional ref update on the remote.
func (c *Client) UpdateRef(u RefUpdate) error {
	body, _ := json.Marshal(u)
	resp, err := c.HTTP.Post(c.BaseURL+"/refs", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update ref %s: %s", u.Name, statusBody(resp))
	}
	return nil
}

func (c *Client) getJSON(path string, v any) error {
	resp, err := c.HTTP.Get(c.BaseURL + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func statusBody(resp *http.Response) string {
	b, _ := io.ReadAll(resp.Body)
	msg := strings.TrimSpace(string(b))
	if msg == "" {
		return resp.Status
	}
	return resp.Status + ": " + msg
}
