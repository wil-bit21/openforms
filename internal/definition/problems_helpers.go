package definition

// problems collects Problems while validating.
type problems []Problem

func (ps *problems) add(path, msg string) {
	*ps = append(*ps, Problem{Path: path, Message: msg})
}

// merge appends the problems of err (if any) with prefix prepended to their paths.
func (ps *problems) merge(prefix string, err error) {
	if err == nil {
		return
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		ps.add(prefix, err.Error())
		return
	}
	for _, p := range ve.Problems {
		ps.add(joinPath(prefix, p.Path), p.Message)
	}
}

func (ps problems) err() error {
	if len(ps) == 0 {
		return nil
	}
	return &ValidationError{Problems: []Problem(ps)}
}

func joinPath(prefix, path string) string {
	switch {
	case prefix == "":
		return path
	case path == "":
		return prefix
	default:
		return prefix + "." + path
	}
}
