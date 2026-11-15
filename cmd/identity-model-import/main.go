// Command identity-model-import converts the browser-identity network of the
// fingerprint-suite project (Apache-2.0) into the model file read by
// internal/identitymodel.
//
//	identity-model-import -in input-network-definition.zip \
//	    -package header-generator@2.1.90 -committed 2026-11-09 \
//	    -out internal/identitymodel/model.json
//
// The output is deterministic: values and leaf entries are sorted, so the same
// input always gives the same bytes. Before writing, the converted model must
// pass the freshness guard (see checkFresh): a stale or incomplete model is
// refused and nothing is written.
//
// With -print and no other flag it lists the shipped model's version, the
// Chrome majors it knows and the default identity distribution, which the
// refresh job diffs before and after an import.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	stealth "github.com/anatolykoptev/go-stealth"
	"github.com/anatolykoptev/go-stealth/internal/identitymodel"
)

type srcNode struct {
	Name         string          `json:"name"`
	ParentNames  []string        `json:"parentNames"`
	PossibleVals []string        `json:"possibleValues"`
	CPT          json.RawMessage `json:"conditionalProbabilities"`
}

func main() {
	in := flag.String("in", "", "upstream input-network-definition.zip")
	commit := flag.String("commit", "", "upstream commit the file was taken from, when known")
	pkg := flag.String("package", "", "npm package the file was taken from, e.g. header-generator@2.1.90")
	committed := flag.String("committed", "", "upstream commit or release date, YYYY-MM-DD")
	out := flag.String("out", "", "model file to write")
	print := flag.Bool("print", false, "print the shipped model's version, Chrome majors and default distribution, then exit")
	flag.Parse()
	if *print {
		if err := printSummary(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "identity-model-import:", err)
			os.Exit(1)
		}
		return
	}
	if *in == "" || *out == "" || *committed == "" || (*commit == "" && *pkg == "") {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*in, *commit, *pkg, *committed, *out); err != nil {
		fmt.Fprintln(os.Stderr, "identity-model-import:", err)
		os.Exit(1)
	}
}

func run(in, commit, pkg, committed, out string) error {
	raw, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	zr, err := zip.NewReader(bytesReaderAt(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	if len(zr.File) == 0 {
		return fmt.Errorf("%s: empty archive", in)
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	var src struct {
		Nodes []srcNode `json:"nodes"`
	}
	if err := json.Unmarshal(body, &src); err != nil {
		return err
	}
	nw := &identitymodel.Network{}
	for _, sn := range src.Nodes {
		nd, err := convertNode(sn)
		if err != nil {
			return fmt.Errorf("node %q: %w", sn.Name, err)
		}
		nw.Nodes = append(nw.Nodes, nd)
	}
	f := identitymodel.File{
		Format: identitymodel.FormatV1,
		Source: identitymodel.Source{
			Project:   "apify/fingerprint-suite",
			Licence:   "Apache-2.0",
			Commit:    commit,
			Package:   pkg,
			Committed: committed,
			File:      "packages/header-generator/src/data_files/input-network-definition.zip",
			SHA256:    hex.EncodeToString(sum[:]),
		},
		Networks: map[string]*identitymodel.Network{"input": nw},
	}
	enc, err := json.Marshal(f)
	if err != nil {
		return err
	}
	parsed, err := identitymodel.Parse(enc)
	if err != nil {
		return fmt.Errorf("converted model does not validate: %w", err)
	}
	if err := checkFresh(parsed, stealth.BuiltinProfiles); err != nil {
		return fmt.Errorf("refusing to import: %w", err)
	}
	return os.WriteFile(out, append(enc, '\n'), 0o644)
}

func convertNode(sn srcNode) (*identitymodel.Node, error) {
	vals := append([]string(nil), sn.PossibleVals...)
	sort.Strings(vals)
	idx := make(map[string]int, len(vals))
	for i, v := range vals {
		idx[v] = i
	}
	cpt, err := convertCPT(sn.CPT, len(sn.ParentNames), idx)
	if err != nil {
		return nil, err
	}
	parents := sn.ParentNames
	if parents == nil {
		parents = []string{}
	}
	return &identitymodel.Node{Name: sn.Name, Parents: parents, Values: vals, CPT: cpt}, nil
}

func convertCPT(raw json.RawMessage, depth int, idx map[string]int) (*identitymodel.CPT, error) {
	if depth == 0 {
		var leaf map[string]float64
		if err := json.Unmarshal(raw, &leaf); err != nil {
			return nil, err
		}
		names := make([]string, 0, len(leaf))
		for k := range leaf {
			names = append(names, k)
		}
		sort.Strings(names)
		c := &identitymodel.CPT{Leaf: []identitymodel.Entry{}}
		for _, k := range names {
			i, ok := idx[k]
			if !ok {
				return nil, fmt.Errorf("leaf value %q is not in possibleValues", k)
			}
			c.Leaf = append(c.Leaf, identitymodel.Entry{Index: i, P: leaf[k]})
		}
		sort.Slice(c.Leaf, func(a, b int) bool { return c.Leaf[a].Index < c.Leaf[b].Index })
		return c, nil
	}
	var lvl struct {
		Deeper map[string]json.RawMessage `json:"deeper"`
		Skip   json.RawMessage            `json:"skip"`
	}
	if err := json.Unmarshal(raw, &lvl); err != nil {
		return nil, err
	}
	c := &identitymodel.CPT{Deeper: map[string]*identitymodel.CPT{}}
	for k, v := range lvl.Deeper {
		sub, err := convertCPT(v, depth-1, idx)
		if err != nil {
			return nil, err
		}
		c.Deeper[k] = sub
	}
	if len(lvl.Skip) > 0 {
		sub, err := convertCPT(lvl.Skip, depth-1, idx)
		if err != nil {
			return nil, err
		}
		c.Skip = sub
	}
	return c, nil
}

type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
