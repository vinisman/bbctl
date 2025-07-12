package utils

func IntersectMap(a, b []string) []string {
	set := make(map[string]struct{}, len(a))
	for _, x := range a {
		set[x] = struct{}{}
	}
	var res []string
	for _, y := range b {
		if _, ok := set[y]; ok {
			res = append(res, y)
		}
	}
	return res
}
