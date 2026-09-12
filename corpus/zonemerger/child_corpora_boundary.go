package zonemerger

import "github.com/memoria-space/meking/corpus"

// ChildCorporaBoundary selects the latest accepted Corpora from one Child Zone.
type ChildCorporaBoundary struct {
	sourceCorporaID corpus.CorporaID
}

func NewChildCorporaBoundary(sourceCorporaID corpus.CorporaID) (ChildCorporaBoundary, error) {
	if err := corpus.ValidateCorporaID(sourceCorporaID); err != nil {
		return ChildCorporaBoundary{}, err
	}
	return ChildCorporaBoundary{sourceCorporaID: sourceCorporaID}, nil
}

func (boundary ChildCorporaBoundary) SourceCorporaID() corpus.CorporaID {
	return boundary.sourceCorporaID
}

func (boundary ChildCorporaBoundary) Merge(
	sourceCorporaID corpus.CorporaID,
) (ChildCorporaBoundary, bool, error) {
	incoming, err := NewChildCorporaBoundary(sourceCorporaID)
	if err != nil {
		return ChildCorporaBoundary{}, false, err
	}
	storedSequence, err := corpus.CorporaIDSequence(boundary.sourceCorporaID)
	if err != nil {
		return ChildCorporaBoundary{}, false, err
	}
	incomingSequence, err := corpus.CorporaIDSequence(incoming.sourceCorporaID)
	if err != nil {
		return ChildCorporaBoundary{}, false, err
	}
	if incomingSequence <= storedSequence {
		return boundary, false, nil
	}
	return incoming, true, nil
}
