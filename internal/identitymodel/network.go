// Package identitymodel samples browser identities from a Bayesian network
// whose probabilities were fitted to real browser traffic.
//
// The network structure and sampling rules follow the open-source
// fingerprint-suite project (Apache-2.0, see NOTICE at the repository root):
// every node holds a conditional probability table keyed by the values of its
// parents, the nodes are listed in sampling order, and a sample may be
// constrained to a subset of each node's values. This package reads the model
// in its own JSON format (see File) and draws from a deterministic generator,
// so one seed always gives one sample, in any language that follows the
// specification in testdata/README.md.
package identitymodel

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

// FormatV1 is the value of File.Format this package reads.
const FormatV1 = "identity-model/1"

// File is the on-disk model: provenance plus one or more named networks.
type File struct {
	// Format is FormatV1.
	Format string `json:"format"`
	// Source records where the probabilities came from.
	Source Source `json:"source"`
	// Networks maps a network name ("input") to its definition.
	Networks map[string]*Network `json:"networks"`
}

// Source is the provenance of a model file.
type Source struct {
	Project string `json:"project"`
	Licence string `json:"licence"`
	// Commit is the upstream git commit, when known.
	Commit string `json:"commit,omitempty"`
	// Package is the published npm package the file was taken from, for
	// example "header-generator@2.1.90", when known.
	Package string `json:"package,omitempty"`
	// Committed is the YYYY-MM-DD of the upstream commit or release.
	Committed string `json:"committed"`
	File      string `json:"file"`   // path of the upstream file
	SHA256    string `json:"sha256"` // of the upstream file as imported
}

// Version is a short, stable label of the model data: the upstream date and
// the short commit hash, or the package version when no commit is known.
func (s Source) Version() string {
	c := s.Commit
	if len(c) > 7 {
		c = c[:7]
	}
	if c == "" {
		_, c, _ = strings.Cut(s.Package, "@")
	}
	return s.Committed + "-" + c
}

// BrowserValues returns every value of node that has a non-zero probability
// somewhere in its table, for example the "chrome/156.0.0.0|2" strings of the
// *BROWSER_HTTP node.
func (n *Network) BrowserValues(node string) map[string]bool {
	out := map[string]bool{}
	nd, ok := n.byName[node]
	if !ok {
		return out
	}
	var walk func(c *CPT)
	walk = func(c *CPT) {
		if c == nil {
			return
		}
		for _, e := range c.Leaf {
			if e.P > 0 {
				out[nd.Values[e.Index]] = true
			}
		}
		for _, sub := range c.Deeper {
			walk(sub)
		}
		walk(c.Skip)
	}
	walk(nd.CPT)
	return out
}

// Network is a list of nodes in sampling order: a node's parents always come
// before it.
type Network struct {
	Nodes []*Node `json:"nodes"`

	byName map[string]*Node
}

// Node is one random variable.
type Node struct {
	Name    string   `json:"name"`
	Parents []string `json:"parents"`
	// Values is every value the node can take; leaf entries index into it.
	Values []string `json:"values"`
	// CPT is the conditional probability table: a tree with one level per
	// parent, in Parents order, ending in a leaf of value probabilities.
	CPT *CPT `json:"cpt"`
}

// CPT is one level of a conditional probability table. A level with parents
// left holds Deeper (one subtree per parent value) and optionally Skip, the
// subtree used when the parent's value has no entry in Deeper. The last level
// holds Leaf: the probability of each value of the node, sorted by value index.
type CPT struct {
	Deeper map[string]*CPT
	Skip   *CPT
	Leaf   []Entry
}

// Entry is one value of a node and its probability.
type Entry struct {
	Index int // into Node.Values
	P     float64
}

// UnmarshalJSON reads a leaf ([[index, p], ...]) or a level ({"d":..., "s":...}).
func (c *CPT) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '[' {
		var raw [][2]float64
		if err := json.Unmarshal(b, &raw); err != nil {
			return err
		}
		c.Leaf = make([]Entry, len(raw))
		for i, r := range raw {
			if r[0] != math.Trunc(r[0]) || r[0] < 0 {
				return fmt.Errorf("identitymodel: leaf index %v is not a non-negative integer", r[0])
			}
			c.Leaf[i] = Entry{Index: int(r[0]), P: r[1]}
		}
		return nil
	}
	var lvl struct {
		D map[string]*CPT `json:"d"`
		S *CPT            `json:"s"`
	}
	if err := json.Unmarshal(b, &lvl); err != nil {
		return err
	}
	c.Deeper, c.Skip = lvl.D, lvl.S
	return nil
}

// MarshalJSON writes the form UnmarshalJSON reads.
func (c *CPT) MarshalJSON() ([]byte, error) {
	if c.Leaf != nil {
		raw := make([][2]float64, len(c.Leaf))
		for i, e := range c.Leaf {
			raw[i] = [2]float64{float64(e.Index), e.P}
		}
		return json.Marshal(raw)
	}
	return json.Marshal(struct {
		D map[string]*CPT `json:"d,omitempty"`
		S *CPT            `json:"s,omitempty"`
	}{c.Deeper, c.Skip})
}

// Index builds the name lookup and checks the structure: unique names,
// parents defined earlier, CPT depth equal to the parent count, leaf indices
// in range and every leaf summing to 1.
func (n *Network) Index() error {
	n.byName = make(map[string]*Node, len(n.Nodes))
	for _, nd := range n.Nodes {
		if _, dup := n.byName[nd.Name]; dup {
			return fmt.Errorf("identitymodel: duplicate node %q", nd.Name)
		}
		for _, p := range nd.Parents {
			if _, ok := n.byName[p]; !ok {
				return fmt.Errorf("identitymodel: node %q: parent %q is not defined before it", nd.Name, p)
			}
		}
		if err := nd.check(nd.CPT, 0); err != nil {
			return fmt.Errorf("identitymodel: node %q: %w", nd.Name, err)
		}
		n.byName[nd.Name] = nd
	}
	return nil
}

func (nd *Node) check(c *CPT, depth int) error {
	if c == nil {
		return errors.New("missing CPT level")
	}
	if depth == len(nd.Parents) {
		if c.Deeper != nil || c.Skip != nil {
			return errors.New("CPT is deeper than the parent count")
		}
		var sum float64
		seen := map[int]bool{}
		for _, e := range c.Leaf {
			if e.Index >= len(nd.Values) {
				return fmt.Errorf("leaf index %d out of range", e.Index)
			}
			if seen[e.Index] {
				return fmt.Errorf("leaf lists value %d twice", e.Index)
			}
			if e.P < 0 {
				return errors.New("negative probability")
			}
			seen[e.Index] = true
			sum += e.P
		}
		if math.Abs(sum-1) > 1e-6 {
			return fmt.Errorf("leaf probabilities sum to %v, want 1", sum)
		}
		return nil
	}
	if c.Leaf != nil {
		return errors.New("CPT is shallower than the parent count")
	}
	for _, sub := range c.Deeper {
		if err := nd.check(sub, depth+1); err != nil {
			return err
		}
	}
	if c.Skip != nil {
		return nd.check(c.Skip, depth+1)
	}
	return nil
}

// Node returns the named node.
func (n *Network) Node(name string) (*Node, bool) {
	nd, ok := n.byName[name]
	return nd, ok
}

// leafFor walks the CPT for the parent values in partial: Deeper[value] when
// present, Skip otherwise. It returns nil when neither exists.
func (nd *Node) leafFor(partial map[string]string) []Entry {
	c := nd.CPT
	for _, p := range nd.Parents {
		if c == nil {
			return nil
		}
		if next, ok := c.Deeper[partial[p]]; ok {
			c = next
		} else {
			c = c.Skip
		}
	}
	if c == nil {
		return nil
	}
	return c.Leaf
}
