package identitymodel

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// Encode writes the model as indented JSON that Parse reads, laid out for a
// line diff: one line per node header and one per CPT subtree, with each leaf
// kept on a single line. The output is deterministic.
func (f *File) Encode() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("{\n")
	if err := writeKV(&b, 1, "format", f.Format, true); err != nil {
		return nil, err
	}
	if err := writeKV(&b, 1, "source", f.Source, true); err != nil {
		return nil, err
	}
	b.WriteString(` "networks": {`)
	names := make([]string, 0, len(f.Networks))
	for n := range f.Networks {
		names = append(names, n)
	}
	sort.Strings(names)
	for i, name := range names {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n  ")
		b.Write(mustJSON(name))
		b.WriteString(": {\"nodes\": [")
		for j, nd := range f.Networks[name].Nodes {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString("\n   {\"name\": ")
			b.Write(mustJSON(nd.Name))
			b.WriteString(", \"parents\": ")
			b.Write(mustJSON(nd.Parents))
			b.WriteString(", \"values\": ")
			b.Write(mustJSON(nd.Values))
			b.WriteString(",\n    \"cpt\": ")
			if err := writeCPT(&b, nd.CPT, 4); err != nil {
				return nil, err
			}
			b.WriteString("}")
		}
		b.WriteString("\n  ]}")
	}
	b.WriteString("\n }\n}\n")
	return b.Bytes(), nil
}

func writeKV(b *bytes.Buffer, depth int, key string, v any, comma bool) error {
	enc, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b.WriteString(strings.Repeat(" ", depth))
	b.Write(mustJSON(key))
	b.WriteString(": ")
	b.Write(enc)
	if comma {
		b.WriteString(",")
	}
	b.WriteString("\n")
	return nil
}

func writeCPT(b *bytes.Buffer, c *CPT, indent int) error {
	if c.Leaf != nil {
		enc, err := c.MarshalJSON()
		if err != nil {
			return err
		}
		b.Write(enc)
		return nil
	}
	pad := strings.Repeat(" ", indent+1)
	b.WriteString("{")
	first := true
	if len(c.Deeper) > 0 {
		keys := make([]string, 0, len(c.Deeper))
		for k := range c.Deeper {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("\n" + pad + `"d": {`)
		for i, k := range keys {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("\n" + pad + " ")
			b.Write(mustJSON(k))
			b.WriteString(": ")
			if err := writeCPT(b, c.Deeper[k], indent+2); err != nil {
				return err
			}
		}
		b.WriteString("\n" + pad + "}")
		first = false
	}
	if c.Skip != nil {
		if !first {
			b.WriteString(",")
		}
		b.WriteString("\n" + pad + `"s": `)
		if err := writeCPT(b, c.Skip, indent+1); err != nil {
			return err
		}
	}
	b.WriteString("}")
	return nil
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
