package ops

import "github.com/DrBopoH/sky-cli/internal/commit"

var commitOp = Operation{
	Name:   "commit",
	Fields: 1,
	Opt:    len(commit.Flags),
	Doc:    docCommit,
	Plan:   planCommit,
}

func planCommit(f []string, _ World) (Result, error) {
	if err := commit.CheckMessage(f[0]); err != nil {
		return Result{}, err
	}
	res := Result{Commit: f[0]}
	for _, fl := range f[1:] {
		if err := res.CommitOpts.Set(fl); err != nil {
			return Result{}, err
		}
	}
	return res, nil
}
