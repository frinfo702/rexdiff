package rex

import "path/filepath"

// NewTestClient returns a client of the session S for the block
// block:self, with the rex in dir.
func NewTestClient(dir string) *Client {
	return &Client{Block: "block:self", session: "S", bin: filepath.Join(dir, "rex")}
}
