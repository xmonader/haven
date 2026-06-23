package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"haven/internal/object"
)

// ClientAuth signs requests so the server can authenticate the actor.
type ClientAuth struct {
	Pub  string // hex ed25519 public key
	Priv ed25519.PrivateKey
}

// Client talks to a haven host at BaseURL.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Auth    *ClientAuth // nil for anonymous requests
}

// NewClient builds a client for a base URL (trailing slash trimmed).
func NewClient(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: http.DefaultClient}
}

// WithAuth attaches signing credentials.
func (c *Client) WithAuth(pub string, priv ed25519.PrivateKey) *Client {
	c.Auth = &ClientAuth{Pub: pub, Priv: priv}
	return c
}

// do builds, signs (over method+path+time+body), and sends a request.
func (c *Client) do(method, path string, body []byte, headers map[string]string) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, r)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if c.Auth != nil {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		nonce := newNonce()
		sig := ed25519.Sign(c.Auth.Priv, canonicalRequest(method, path, ts, bodyHash(body), nonce))
		req.Header.Set(HdrPub, c.Auth.Pub)
		req.Header.Set(HdrTime, ts)
		req.Header.Set(HdrNonce, nonce)
		req.Header.Set(HdrSig, hex.EncodeToString(sig))
	}
	return c.HTTP.Do(req)
}

// newNonce returns a random 128-bit hex token, unique per request.
func newNonce() string {
	var b [16]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
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
	resp, err := c.do(http.MethodGet, "/objects/"+hash, nil, nil)
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
	resp, err := c.do(http.MethodPut, "/objects/"+hash, content, map[string]string{HeaderType: string(typ)})
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
	resp, err := c.do(http.MethodPost, "/refs", body, map[string]string{"Content-Type": "application/json"})
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
	resp, err := c.do(http.MethodGet, path, nil, nil)
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
