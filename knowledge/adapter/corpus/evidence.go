package corpus

import (
	"context"
	"errors"
	"fmt"
	"sort"

	corpusdomain "github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/zone"
)

type Corpora interface {
	TextUnitLocations(
		ctx context.Context,
		corporaID corpusdomain.CorporaID,
		textUnitIDs []textunits.TextUnitID,
	) ([]corpusdomain.TextUnitLocation, error)
}

type Messages interface {
	Read(ctx context.Context, ids []string) ([]message.Occurrence, error)
}

type EvidenceVerifier struct {
	corpora  Corpora
	messages Messages
}

func NewEvidenceVerifier(corpora Corpora, messages Messages) (*EvidenceVerifier, error) {
	if corpora == nil {
		return nil, errors.New("create Knowledge Evidence verifier: Corpus is required")
	}
	if messages == nil {
		return nil, errors.New("create Knowledge Evidence verifier: Messages are required")
	}
	return &EvidenceVerifier{
		corpora: corpora, messages: messages,
	}, nil
}

func (verifier *EvidenceVerifier) VerifyEvidence(
	ctx context.Context,
	evidence []provenance.Evidence,
) error {
	if verifier == nil || verifier.corpora == nil || verifier.messages == nil {
		return errors.New("Knowledge Evidence verifier is not configured")
	}
	byZone := make(map[string]map[string][]textunits.TextUnitID)
	messagesByZone := make(map[string]map[string]string)
	for _, item := range evidence {
		switch source := item.Source.(type) {
		case provenance.CorporaSource:
			byCorpora := byZone[item.ZoneID]
			if byCorpora == nil {
				byCorpora = make(map[string][]textunits.TextUnitID)
				byZone[item.ZoneID] = byCorpora
			}
			byCorpora[source.CorporaID] = append(
				byCorpora[source.CorporaID],
				textunits.TextUnitID(item.TextUnitID),
			)
		case provenance.MessageSource:
			byMessage := messagesByZone[item.ZoneID]
			if byMessage == nil {
				byMessage = make(map[string]string)
				messagesByZone[item.ZoneID] = byMessage
			}
			if existing, found := byMessage[source.MessageID]; found && existing != item.TextUnitID {
				return fmt.Errorf("%w: one Message cannot identify two TextUnits", provenance.ErrInvalidSource)
			}
			byMessage[source.MessageID] = item.TextUnitID
		default:
			return fmt.Errorf("%w: unsupported Evidence Source %T", provenance.ErrInvalidSource, item.Source)
		}
	}
	zoneSet := make(map[string]struct{}, len(byZone)+len(messagesByZone))
	for zoneID := range byZone {
		zoneSet[zoneID] = struct{}{}
	}
	for zoneID := range messagesByZone {
		zoneSet[zoneID] = struct{}{}
	}
	zoneIDs := make([]string, 0, len(zoneSet))
	for zoneID := range zoneSet {
		zoneIDs = append(zoneIDs, zoneID)
	}
	sort.Strings(zoneIDs)
	for _, zoneID := range zoneIDs {
		owner, err := zone.ParseID(zoneID)
		if err != nil {
			return fmt.Errorf("%w: Evidence ZoneID %q is invalid", provenance.ErrInvalidSource, zoneID)
		}
		ownerContext, err := zone.RouteContext(ctx, owner)
		if err != nil {
			return err
		}
		byCorpora := byZone[zoneID]
		corporaIDs := make([]string, 0, len(byCorpora))
		for corporaID := range byCorpora {
			corporaIDs = append(corporaIDs, corporaID)
		}
		sort.Strings(corporaIDs)
		for _, corporaID := range corporaIDs {
			requested := byCorpora[corporaID]
			locations, err := verifier.corpora.TextUnitLocations(
				ownerContext,
				corpusdomain.CorporaID(corporaID),
				requested,
			)
			if err != nil {
				return fmt.Errorf("verify Knowledge Evidence in Zone %q Corpora %q: %w", zoneID, corporaID, err)
			}
			found := make(map[textunits.TextUnitID]struct{}, len(locations))
			for _, location := range locations {
				if string(location.CorporaID) != corporaID {
					return fmt.Errorf("%w: Corpus returned Evidence from another Corpora", provenance.ErrInvalidSource)
				}
				found[location.TextUnit.ID()] = struct{}{}
			}
			for _, textUnitID := range requested {
				if _, exists := found[textUnitID]; !exists {
					return fmt.Errorf(
						"%w: Evidence %s/%s/%s does not exist",
						provenance.ErrInvalidSource,
						zoneID,
						corporaID,
						textUnitID,
					)
				}
			}
		}
		byMessage := messagesByZone[zoneID]
		if len(byMessage) == 0 {
			continue
		}
		messageIDs := make([]string, 0, len(byMessage))
		for messageID := range byMessage {
			messageIDs = append(messageIDs, messageID)
		}
		sort.Strings(messageIDs)
		occurrences, err := verifier.messages.Read(ownerContext, messageIDs)
		if err != nil {
			return fmt.Errorf("verify Knowledge Message Evidence in Zone %q: %w", zoneID, err)
		}
		if len(occurrences) != len(messageIDs) {
			return fmt.Errorf("%w: Corpus returned incomplete Message Evidence", provenance.ErrInvalidSource)
		}
		for _, occurrence := range occurrences {
			expected, exists := byMessage[occurrence.Message.ID]
			if !exists || string(occurrence.Message.TextUnit.ID) != expected {
				return fmt.Errorf(
					"%w: Message Evidence %s/%s/%s does not exist",
					provenance.ErrInvalidSource,
					zoneID,
					occurrence.Message.ID,
					expected,
				)
			}
		}
	}
	return nil
}
