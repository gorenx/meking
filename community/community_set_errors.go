package community

import "errors"

var (
	// ErrInvalidDetectionGraph identifies a Community detection source whose
	// Knowledge references, endpoints, weights, or deterministic ordering cannot
	// form one complete detector input.
	ErrInvalidDetectionGraph = errors.New("invalid Community detection graph")
	// ErrInvalidCommunitySet identifies an ID, knowledge reference, hierarchy,
	// membership, timestamp, or ordering rule that cannot form a CommunitySet.
	ErrInvalidCommunitySet = errors.New("invalid CommunitySet")
	// ErrCommunitySetNotFound identifies an immutable CommunitySet ID that is
	// absent from the Community Store.
	ErrCommunitySetNotFound = errors.New("CommunitySet not found")
	// ErrCommunitySetConflict identifies an attempt to reuse one CommunitySet ID
	// for another immutable set.
	ErrCommunitySetConflict = errors.New("CommunitySet identity conflict")
	// ErrCommunityDataIntegrity identifies persisted Community rows that cannot
	// reconstruct one valid immutable CommunitySet.
	ErrCommunityDataIntegrity = errors.New("Community data integrity failure")
	// ErrCommunityStorageBusy identifies SQLite lock contention that exhausted
	// the adapter's configured wait and can be retried without changing input.
	ErrCommunityStorageBusy = errors.New("Community storage busy")
)
