package zone

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DefinitionStore persists and resolves Zone definitions for Catalog.
type DefinitionStore interface {
	Create(ctx context.Context, definition Definition) error
	Resolve(ctx context.Context, id ID) (Definition, error)
	ResolveRoot(ctx context.Context, userID string) (Definition, error)
	List(ctx context.Context) ([]Definition, error)
}

// Resolver is the read contract consumed by Zone-scoped applications.
type Resolver interface {
	Resolve(ctx context.Context, id ID) (Definition, error)
	ResolveDirectParent(ctx context.Context, childID ID) (ID, error)
}

// CatalogDependencies contains the outbound contracts required by Catalog.
type CatalogDependencies struct {
	Definitions DefinitionStore
}

// Catalog owns Zone creation and direct Parent resolution.
type Catalog struct {
	definitions DefinitionStore
	creation    sync.Mutex
}

// NewCatalog creates the Zone Catalog.
func NewCatalog(dependencies CatalogDependencies) (*Catalog, error) {
	if dependencies.Definitions == nil {
		return nil, fmt.Errorf("%w: Definition Store is required", ErrNotReady)
	}
	return &Catalog{definitions: dependencies.Definitions}, nil
}

// CreateRoot creates one Root Zone.
func (catalog *Catalog) CreateRoot(ctx context.Context) (Definition, error) {
	return catalog.createRoot(ctx, "")
}

func (catalog *Catalog) createRoot(ctx context.Context, userID string) (Definition, error) {
	if err := ctx.Err(); err != nil {
		return Definition{}, err
	}
	id, err := NewID()
	if err != nil {
		return Definition{}, err
	}
	definition, err := NewRootDefinition(id, userID, time.Now().UTC())
	if err != nil {
		return Definition{}, err
	}
	if err := catalog.definitions.Create(ctx, definition); err != nil {
		return Definition{}, fmt.Errorf("create Root Zone: %w", err)
	}
	return definition, nil
}

// CreateChild creates one direct Child of a Root Zone.
func (catalog *Catalog) CreateChild(ctx context.Context, parentID ID) (Definition, error) {
	parent, err := catalog.resolveRoot(ctx, parentID)
	if err != nil {
		return Definition{}, err
	}
	id, err := NewID()
	if err != nil {
		return Definition{}, err
	}
	return catalog.createChild(ctx, parent, id)
}

func (catalog *Catalog) createChild(
	ctx context.Context,
	parent Definition,
	childID ID,
) (Definition, error) {
	definition, err := NewChildDefinition(childID, parent.ID, time.Now().UTC())
	if err != nil {
		return Definition{}, err
	}
	if err := catalog.definitions.Create(ctx, definition); err != nil {
		return Definition{}, fmt.Errorf("create Child Zone: %w", err)
	}
	return definition, nil
}

// Root returns the Root Zone assigned to a User, creating it when this is the
// first request from that User.
func (catalog *Catalog) Root(ctx context.Context, userID string) (Definition, error) {
	if err := validateUserID(userID); err != nil {
		return Definition{}, err
	}
	catalog.creation.Lock()
	defer catalog.creation.Unlock()

	root, err := catalog.definitions.ResolveRoot(ctx, userID)
	if err == nil {
		return requireUserRoot(root, userID)
	}
	if !errors.Is(err, ErrNotFound) {
		return Definition{}, err
	}
	root, createErr := catalog.createRoot(ctx, userID)
	if createErr == nil {
		return root, nil
	}
	root, resolveErr := catalog.definitions.ResolveRoot(ctx, userID)
	if resolveErr != nil {
		return Definition{}, errors.Join(createErr, resolveErr)
	}
	return requireUserRoot(root, userID)
}

// EnsureChild returns the Child Zone with the requested identity, creating it
// under the direct Root Parent when it does not exist.
func (catalog *Catalog) EnsureChild(
	ctx context.Context,
	parentID ID,
	childID ID,
) (Definition, error) {
	catalog.creation.Lock()
	defer catalog.creation.Unlock()

	parent, err := catalog.resolveRoot(ctx, parentID)
	if err != nil {
		return Definition{}, err
	}
	existing, err := catalog.Resolve(ctx, childID)
	switch {
	case err == nil:
		return requireChild(existing, parent.ID)
	case err != nil && !errors.Is(err, ErrNotFound):
		return Definition{}, err
	}
	definition, createErr := catalog.createChild(ctx, parent, childID)
	if createErr != nil {
		existing, resolveErr := catalog.Resolve(ctx, childID)
		if resolveErr != nil {
			return Definition{}, errors.Join(createErr, resolveErr)
		}
		return requireChild(existing, parent.ID)
	}
	return definition, nil
}

func (catalog *Catalog) resolveRoot(ctx context.Context, rootID ID) (Definition, error) {
	root, err := catalog.Resolve(ctx, rootID)
	if err != nil {
		return Definition{}, err
	}
	if root.Role != RoleRoot {
		return Definition{}, ErrChildDepth
	}
	return root, nil
}

func requireUserRoot(definition Definition, userID string) (Definition, error) {
	if err := validateDefinition(definition); err != nil {
		return Definition{}, fmt.Errorf("resolve User Root Zone: %w", err)
	}
	if definition.Role != RoleRoot || definition.UserID != userID {
		return Definition{}, fmt.Errorf(
			"%w: User Root Zone does not match User %q",
			ErrInvalidDefinition,
			userID,
		)
	}
	return definition, nil
}

func requireChild(definition Definition, parentID ID) (Definition, error) {
	actualParent, ok := definition.Parent()
	if !ok {
		return Definition{}, fmt.Errorf(
			"%w: Zone %q already exists as %s",
			ErrInvalidDefinition,
			definition.ID,
			definition.Role,
		)
	}
	if actualParent != parentID {
		return Definition{}, fmt.Errorf(
			"%w: Child Zone %q has Parent %q instead of %q",
			ErrParentConflict,
			definition.ID,
			actualParent,
			parentID,
		)
	}
	return definition, nil
}

// Resolve returns one exact Zone definition.
func (catalog *Catalog) Resolve(ctx context.Context, id ID) (Definition, error) {
	if err := ctx.Err(); err != nil {
		return Definition{}, err
	}
	validated, err := ParseID(string(id))
	if err != nil {
		return Definition{}, err
	}
	definition, err := catalog.definitions.Resolve(ctx, validated)
	if err != nil {
		return Definition{}, err
	}
	if err := validateDefinition(definition); err != nil {
		return Definition{}, fmt.Errorf("resolve Zone %q: %w", validated, err)
	}
	if definition.ID != validated {
		return Definition{}, fmt.Errorf(
			"%w: resolved Zone ID %q does not match requested ID %q",
			ErrInvalidDefinition,
			definition.ID,
			validated,
		)
	}
	return definition, nil
}

// ResolveDirectParent returns the direct Parent of one Child Zone.
func (catalog *Catalog) ResolveDirectParent(ctx context.Context, childID ID) (ID, error) {
	child, err := catalog.Resolve(ctx, childID)
	if err != nil {
		return "", err
	}
	parentID, ok := child.Parent()
	if !ok {
		return "", ErrParentRequired
	}
	parent, err := catalog.resolveRoot(ctx, parentID)
	if err != nil {
		return "", err
	}
	return parent.ID, nil
}

// List returns all Zone definitions visible to the application runtime.
func (catalog *Catalog) List(ctx context.Context) ([]Definition, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	definitions, err := catalog.definitions.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Definition, len(definitions))
	for index, definition := range definitions {
		if err := validateDefinition(definition); err != nil {
			return nil, fmt.Errorf("list Zones: definition %d: %w", index, err)
		}
		result[index] = definition
	}
	return result, nil
}
