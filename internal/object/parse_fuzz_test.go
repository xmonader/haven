package object

import "testing"

// The server parses object bytes received over the wire; malformed input must
// produce an error, never a panic.
func FuzzParseCommit(f *testing.F) {
	f.Add([]byte("tree t\nparent p\nauthor N <e> 123\n\nmessage"))
	f.Add([]byte(""))
	f.Add([]byte("tree\n\n"))
	f.Add([]byte("garbage without structure"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseCommit(data)
	})
}

func FuzzParseTree(f *testing.F) {
	f.Add([]byte("100644 blob aaa\tfile.txt\n"))
	f.Add([]byte(""))
	f.Add([]byte("\x00\x01\x02 bad"))
	f.Add([]byte("100644 blob\tnoHashColumn\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseTree(data)
	})
}
