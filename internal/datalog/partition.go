package datalog

func PartitionRules(rules []Rule) (recursive, nonRecursive []Rule) {
	heads := make(map[string]bool)
	for _, r := range rules {
		heads[r.Head.Rel] = true
	}

	for _, r := range rules {
		isRecursive := false
		for _, atom := range r.Body {
			if heads[atom.Rel] {
				isRecursive = true
				break
			}
		}
		if isRecursive {
			recursive = append(recursive, r)
		} else {
			nonRecursive = append(nonRecursive, r)
		}
	}
	return
}
