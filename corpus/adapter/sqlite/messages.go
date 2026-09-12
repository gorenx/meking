package sqlite

import (
	"context"
	"fmt"
	"math"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/zone"
)

var _ message.Entries = (*Store)(nil)

func (store *Store) Find(ctx context.Context, ids []string) ([]message.Occurrence, error) {
	statements, zoneID, err := store.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := statements.ListMessages(ctx, db.ListMessagesParams{
		ZoneID:     zoneID,
		MessageIds: append([]string(nil), ids...),
	})
	if err != nil {
		return nil, classifySQLite("read Messages", err)
	}
	result := make([]message.Occurrence, len(rows))
	for index, row := range rows {
		occurrence, err := restoreMessageFields(
			row.ID,
			row.Role,
			row.Position,
			row.TextUnitID,
			row.TextUnitText,
		)
		if err != nil {
			return nil, err
		}
		result[index] = occurrence
	}
	return result, nil
}

func (store *Store) LastPosition(ctx context.Context) (position uint64, found bool, err error) {
	statements, zoneID, err := store.reader(ctx)
	if err != nil {
		return 0, false, err
	}
	last, err := statements.LastMessagePosition(ctx, zoneID)
	if err != nil {
		return 0, false, classifySQLite("read last Message position", err)
	}
	if last < 0 {
		return 0, false, nil
	}
	return uint64(last), true, nil
}

func (store *Store) Save(ctx context.Context, occurrences []message.Occurrence) error {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	zoneIDValue := string(zoneID)
	return store.write(ctx, zoneIDValue, func(statements statements) error {
		for _, occurrence := range occurrences {
			if err := message.ValidateOccurrence(occurrence); err != nil {
				return err
			}
			if occurrence.Position > math.MaxInt64 {
				return fmt.Errorf("%w: Message Position exceeds SQLite integer range", corpus.ErrCorpusDataIntegrity)
			}
			if err := saveTextUnit(ctx, statements, zoneIDValue, occurrence.Message.TextUnit); err != nil {
				return err
			}
			inserted, err := statements.SaveMessage(ctx, db.SaveMessageParams{
				ZoneID:     zoneIDValue,
				ID:         occurrence.Message.ID,
				Role:       occurrence.Message.Role,
				Position:   int64(occurrence.Position),
				TextUnitID: string(occurrence.Message.TextUnit.ID),
			})
			if err != nil {
				return classifySQLite("save Message", err)
			}
			if inserted != 1 {
				return fmt.Errorf("%w: Message row was not inserted", corpus.ErrCorpusDataIntegrity)
			}
		}
		return nil
	})
}

func restoreMessageFields(
	id string,
	role string,
	position int64,
	textUnitID string,
	text string,
) (message.Occurrence, error) {
	if position < 0 {
		return message.Occurrence{}, fmt.Errorf("%w: negative Message position", corpus.ErrCorpusDataIntegrity)
	}
	value, err := message.New(id, role, text)
	if err != nil {
		return message.Occurrence{}, fmt.Errorf("%w: restore Message: %v", corpus.ErrCorpusDataIntegrity, err)
	}
	if value.TextUnit.ID != textunits.TextUnitID(textUnitID) {
		return message.Occurrence{}, fmt.Errorf("%w: Message TextUnit identity differs from content", corpus.ErrCorpusDataIntegrity)
	}
	return message.Occurrence{Message: value, Position: uint64(position)}, nil
}
