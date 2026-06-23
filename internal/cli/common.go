package cli

import (
	"haven/internal/object"
	"haven/internal/repo"
)

// openRepo opens the repository containing the current directory and returns
// it along with an object store.
func openRepo() (*repo.Repo, *object.Store, error) {
	r, err := repo.Open(".")
	if err != nil {
		return nil, nil, err
	}
	return r, object.NewStore(r.DB), nil
}
