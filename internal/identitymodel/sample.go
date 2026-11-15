package identitymodel

// Allow reports whether value may be taken by node given the values already
// chosen for earlier nodes. A nil Allow permits everything.
type Allow func(node string, partial map[string]string, value string) bool

func (a Allow) ok(node string, partial map[string]string, value string) bool {
	return a == nil || a(node, partial, value)
}

// Sample draws one assignment of every node, walking the nodes in order. At
// each node the candidates are the values with a probability in the node's
// table that allow permits; one is chosen in proportion to its probability.
// When a later node has no candidate left, the value chosen before it is
// banned and the node is drawn again from what remains, backtracking as far
// as needed. It returns false when no assignment satisfies allow.
//
// Candidates are visited in the order of the table (sorted by value index),
// and each attempt consumes exactly one Float64 from r, so a seed fixes the
// result.
func (n *Network) Sample(r *Rand, allow Allow) (map[string]string, bool) {
	partial := make(map[string]string, len(n.Nodes))
	if n.sample(r, allow, partial, 0) {
		return partial, true
	}
	return nil, false
}

func (n *Network) sample(r *Rand, allow Allow, partial map[string]string, depth int) bool {
	if depth == len(n.Nodes) {
		return true
	}
	nd := n.Nodes[depth]
	banned := map[int]bool{}
	for {
		leaf := nd.leafFor(partial)
		var cand []Entry
		var total float64
		for _, e := range leaf {
			if banned[e.Index] || !allow.ok(nd.Name, partial, nd.Values[e.Index]) {
				continue
			}
			cand = append(cand, e)
			total += e.P
		}
		if len(cand) == 0 {
			delete(partial, nd.Name)
			return false
		}
		anchor := r.Float64() * total
		chosen := cand[len(cand)-1]
		var cum float64
		for _, e := range cand {
			cum += e.P
			if cum > anchor {
				chosen = e
				break
			}
		}
		partial[nd.Name] = nd.Values[chosen.Index]
		if n.sample(r, allow, partial, depth+1) {
			return true
		}
		banned[chosen.Index] = true
	}
}

// Outcome is one complete assignment with its exact probability.
type Outcome struct {
	Values map[string]string
	P      float64
}

// Distribution returns every assignment Sample can produce under allow with
// the probability Sample gives it, by exhaustive enumeration. Dead branches
// are removed and the survivors renormalised level by level, which is what
// banning a failed value and drawing again amounts to.
func (n *Network) Distribution(allow Allow) []Outcome {
	var out []Outcome
	n.enumerate(allow, map[string]string{}, 0, 1, &out)
	return out
}

func (n *Network) live(allow Allow, partial map[string]string, depth int) bool {
	if depth == len(n.Nodes) {
		return true
	}
	nd := n.Nodes[depth]
	defer delete(partial, nd.Name)
	for _, e := range nd.leafFor(partial) {
		v := nd.Values[e.Index]
		if !allow.ok(nd.Name, partial, v) {
			continue
		}
		partial[nd.Name] = v
		if n.live(allow, partial, depth+1) {
			return true
		}
	}
	return false
}

func (n *Network) enumerate(allow Allow, partial map[string]string, depth int, p float64, out *[]Outcome) {
	if depth == len(n.Nodes) {
		cp := make(map[string]string, len(partial))
		for k, v := range partial {
			cp[k] = v
		}
		*out = append(*out, Outcome{Values: cp, P: p})
		return
	}
	nd := n.Nodes[depth]
	var cand []Entry
	var total float64
	for _, e := range nd.leafFor(partial) {
		v := nd.Values[e.Index]
		if !allow.ok(nd.Name, partial, v) {
			continue
		}
		partial[nd.Name] = v
		alive := n.live(allow, partial, depth+1)
		delete(partial, nd.Name)
		if alive {
			cand = append(cand, e)
			total += e.P
		}
	}
	for _, e := range cand {
		partial[nd.Name] = nd.Values[e.Index]
		n.enumerate(allow, partial, depth+1, p*e.P/total, out)
	}
	delete(partial, nd.Name)
}
