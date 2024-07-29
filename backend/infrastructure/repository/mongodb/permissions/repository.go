package permissions

import (
	"cmp"
	"errors"
	"slices"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type PermissionsRepository struct {
	collection []permission.Permission
}

var _ permission.Repository = &PermissionsRepository{}

func NewRepository() *PermissionsRepository {
	slices.SortStableFunc(
		collection,
		func(a permission.Permission, b permission.Permission) int {
			return cmp.Compare(a.Value, b.Value)
		},
	)

	return &PermissionsRepository{
		collection: collection,
	}
}

func (r *PermissionsRepository) GetAll() []permission.Permission {
	return r.collection
}

func (r *PermissionsRepository) GetOne(value string) (permission.Permission, error) {
	index, found := slices.BinarySearchFunc(
		r.collection,
		permission.Permission{
			Value: value,
		},
		func(a permission.Permission, b permission.Permission) int {
			return cmp.Compare(a.Value, b.Value)
		},
	)
	if !found {
		return permission.Permission{}, domain.ErrNotExists
	}

	return r.collection[index], nil
}

func (r *PermissionsRepository) Get(values []string) ([]permission.Permission, error) {
	result := make([]permission.Permission, 0, len(values))

	for i := range values {
		p, err := r.GetOne(values[i])
		if err != nil && errors.Is(err, domain.ErrNotExists) {
			continue
		} else if err != nil {
			return nil, err
		}

		result = append(result, p)
	}

	return result, nil
}
