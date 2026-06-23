package object

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// CommitObj is a snapshot: a tree plus lineage and authorship metadata.
type CommitObj struct {
	Tree    string
	Parents []string // zero (root) or more
	Author  string   // display name
	Email   string
	When    int64 // unix seconds
	Message string
}

// SerializeCommit renders a commit to its canonical form:
//
//	tree <hash>
//	parent <hash>        (repeated)
//	author <name> <email> <when>
//	<blank line>
//	<message>
func SerializeCommit(c CommitObj) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "tree %s\n", c.Tree)
	for _, p := range c.Parents {
		fmt.Fprintf(&b, "parent %s\n", p)
	}
	fmt.Fprintf(&b, "author %s <%s> %d\n", c.Author, c.Email, c.When)
	b.WriteString("\n")
	b.WriteString(c.Message)
	return b.Bytes()
}

// ParseCommit decodes a commit payload.
func ParseCommit(payload []byte) (CommitObj, error) {
	var c CommitObj
	header, msg, _ := strings.Cut(string(payload), "\n\n")
	c.Message = msg
	for _, line := range strings.Split(header, "\n") {
		switch {
		case strings.HasPrefix(line, "tree "):
			c.Tree = line[len("tree "):]
		case strings.HasPrefix(line, "parent "):
			c.Parents = append(c.Parents, line[len("parent "):])
		case strings.HasPrefix(line, "author "):
			if err := parseAuthor(line[len("author "):], &c); err != nil {
				return c, err
			}
		}
	}
	if c.Tree == "" {
		return c, fmt.Errorf("commit missing tree")
	}
	return c, nil
}

// parseAuthor parses "<name> <email> <when>".
func parseAuthor(s string, c *CommitObj) error {
	open := strings.LastIndex(s, " <")
	close := strings.Index(s, "> ")
	if open < 0 || close < 0 || close < open {
		return fmt.Errorf("malformed author line: %q", s)
	}
	c.Author = s[:open]
	c.Email = s[open+2 : close]
	when, err := strconv.ParseInt(strings.TrimSpace(s[close+2:]), 10, 64)
	if err != nil {
		return fmt.Errorf("malformed author time: %w", err)
	}
	c.When = when
	return nil
}

// PutCommit serializes and stores a commit, returning its hash.
func (s *Store) PutCommit(c CommitObj) (string, error) {
	return s.Put(Commit, SerializeCommit(c))
}

// GetCommit fetches and parses a commit object.
func (s *Store) GetCommit(h string) (CommitObj, error) {
	t, payload, err := s.Get(h)
	if err != nil {
		return CommitObj{}, err
	}
	if t != Commit {
		return CommitObj{}, fmt.Errorf("object %s is %s, want commit", h, t)
	}
	return ParseCommit(payload)
}
