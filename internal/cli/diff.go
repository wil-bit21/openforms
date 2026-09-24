package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definition"
)

func NewDiffCmd(d Deps) *cobra.Command {
	var rf remoteFlags
	cmd := &cobra.Command{
		Use:          "diff",
		Short:        "Show which definitions differ between local files and the server",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, err := LoadProjectConfig(d.WorkDir)
			if err != nil {
				return err
			}
			lb, problems, err := LoadLocal(d.resolveDir(rf.dir, pc))
			if err != nil {
				return err
			}
			if len(problems) > 0 {
				printProblems(d.Err, problems)
				return ErrProblems
			}
			server, key, err := d.resolveRemote(rf, pc)
			if err != nil {
				return err
			}
			remote, err := d.NewClient(server, key).Export(cmd.Context())
			if err != nil {
				return explainRemoteError(d.Err, err)
			}
			lines, err := diffBundles(lb.Bundle, remote)
			if err != nil {
				return err
			}
			if len(lines) == 0 {
				fmt.Fprintln(d.Out, "No differences.")
				return nil
			}
			for _, l := range lines {
				fmt.Fprintln(d.Out, l)
			}
			return nil
		},
	}
	addRemoteFlags(cmd, &rf)
	return cmd
}

type defKey struct{ kind, slug string }

func bundleHashes(b client.Bundle) (map[defKey]string, error) {
	m := map[defKey]string{}
	for _, f := range b.Forms {
		_, h, err := definition.Canonical(f)
		if err != nil {
			return nil, err
		}
		m[defKey{"form", f.Slug}] = h
	}
	for _, w := range b.Workflows {
		_, h, err := definition.Canonical(w)
		if err != nil {
			return nil, err
		}
		m[defKey{"workflow", w.Slug}] = h
	}
	return m, nil
}

// diffBundles compares by canonical hash; unchanged definitions are omitted.
func diffBundles(local, remote client.Bundle) ([]string, error) {
	l, err := bundleHashes(local)
	if err != nil {
		return nil, err
	}
	r, err := bundleHashes(remote)
	if err != nil {
		return nil, err
	}
	keys := map[defKey]bool{}
	for k := range l {
		keys[k] = true
	}
	for k := range r {
		keys[k] = true
	}
	sorted := make([]defKey, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].kind != sorted[j].kind {
			return sorted[i].kind < sorted[j].kind
		}
		return sorted[i].slug < sorted[j].slug
	})
	var out []string
	for _, k := range sorted {
		lh, inLocal := l[k]
		rh, inRemote := r[k]
		switch {
		case inLocal && !inRemote:
			out = append(out, fmt.Sprintf("+ %s %s (local only)", k.kind, k.slug))
		case !inLocal && inRemote:
			out = append(out, fmt.Sprintf("- %s %s (remote only)", k.kind, k.slug))
		case lh != rh:
			out = append(out, fmt.Sprintf("~ %s %s", k.kind, k.slug))
		}
	}
	return out, nil
}
