package corpus

import "errors"

var (
	// ErrInvalidCorpus identifies a Corpus, Document occurrence, TextUnit
	// occurrence, ID, or activation request that violates Corpus rules.
	ErrInvalidCorpus = errors.New("invalid Corpus")
	// ErrContentConflict identifies one content-derived Document or TextUnit ID
	// that is already associated with different bytes.
	ErrContentConflict = errors.New("Corpus content identity conflict")
	// ErrCorporaNotFound identifies a requested immutable or current Corpus
	// that does not exist.
	ErrCorporaNotFound = errors.New("Corpora not found")
	// ErrCorpusDataIntegrity identifies persisted rows that cannot reconstruct a
	// valid Corpus or immutable content value.
	ErrCorpusDataIntegrity = errors.New("Corpus data integrity failure")
	// ErrCorpusStorageBusy identifies SQLite lock contention that exceeded the
	// configured wait. A caller may retry without changing the Corpus.
	ErrCorpusStorageBusy = errors.New("Corpus storage busy")
	// ErrCorporaPreparationConflict identifies a replay or write that conflicts
	// with the active Corpora build or its persisted Text chunking progress.
	ErrCorporaPreparationConflict = errors.New("Corpora preparation conflict")
	// ErrCorporaAwaitingActivation prevents a completed candidate from being
	// mutated before its vector verification activates it.
	ErrCorporaAwaitingActivation = errors.New("Corpora awaiting activation")
	// ErrCorporaActivationConflict identifies incompatible readiness or a
	// conflicting durable activation result for the same Corpus.
	ErrCorporaActivationConflict = errors.New("Corpora activation conflict")
)
