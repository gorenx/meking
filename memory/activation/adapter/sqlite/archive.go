package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/memoria-space/meking/memory/activation"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
)

func (database *Database) InsertModel(ctx context.Context, model activation.Model) error {
	executor, err := transactionsqlite.Current(ctx, database.database)
	if err != nil {
		return err
	}
	parameters, err := json.Marshal(model.Config().Parameters)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO activation_models (model_id,formula,parameters) VALUES (?,?,?)`, model.ID(), activation.ModelFormula, string(parameters))
	return classify(err)
}

func (database *Database) InsertProtocol(ctx context.Context, protocol activation.Protocol) error {
	executor, err := transactionsqlite.Current(ctx, database.database)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO activation_protocols (digest,version,language,prompt,input_schema,output_schema) VALUES (?,?,?,?,?,?)`,
		protocol.Digest(), protocol.Version(), protocol.Language(), protocol.Prompt(), protocol.InputSchema(), protocol.OutputSchema())
	return classify(err)
}

func (database *Database) ReadModel(ctx context.Context, id string) (activation.Model, bool, error) {
	executor, err := database.reader(ctx)
	if err != nil {
		return activation.Model{}, false, err
	}
	var formula, parameters string
	err = executor.QueryRowContext(ctx, `SELECT formula,parameters FROM activation_models WHERE model_id=?`, id).Scan(&formula, &parameters)
	if errors.Is(err, sql.ErrNoRows) {
		return activation.Model{}, false, nil
	}
	if err != nil {
		return activation.Model{}, false, classify(err)
	}
	var values []float64
	if err := decode(parameters, &values); err != nil {
		return activation.Model{}, false, err
	}
	model, err := activation.RestoreModel(id, formula, values)
	if err != nil {
		return activation.Model{}, false, err
	}
	return model, true, nil
}

func (database *Database) ReadProtocol(ctx context.Context, id string) (activation.Protocol, bool, error) {
	executor, err := database.reader(ctx)
	if err != nil {
		return activation.Protocol{}, false, err
	}
	var version, language, prompt, input, output string
	err = executor.QueryRowContext(ctx, `SELECT version,language,prompt,input_schema,output_schema FROM activation_protocols WHERE digest=?`, id).Scan(&version, &language, &prompt, &input, &output)
	if errors.Is(err, sql.ErrNoRows) {
		return activation.Protocol{}, false, nil
	}
	if err != nil {
		return activation.Protocol{}, false, classify(err)
	}
	protocol, err := activation.RestoreProtocol(id, version, language, prompt, input, output)
	if err != nil {
		return activation.Protocol{}, false, err
	}
	return protocol, true, nil
}
