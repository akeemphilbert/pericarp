package projection

import "errors"

var (
	ErrTypeNotRegistered     = errors.New("resource type not registered")
	ErrDuplicateType         = errors.New("resource type already registered")
	ErrAbstractRequiresTable = errors.New("abstract resource type must specify a table name")
	ErrCannotOverrideTable   = errors.New("concrete subtype cannot override parent table name")
	ErrParentNotRegistered   = errors.New("parent resource type not registered")
	ErrParentNotAbstract     = errors.New("parent resource type is not abstract")
	ErrNestedAbstract        = errors.New("abstract type cannot be a subtype of another type")
)
