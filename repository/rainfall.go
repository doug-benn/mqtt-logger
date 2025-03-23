package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/doug-benn/mqtt-logger/model"
)

type RainfallRepo struct {
	db *sql.DB
}

func NewRainfallRepository(db *sql.DB) RainfallRepository {
	return &RainfallRepo{
		db: db,
	}
}

type RainfallRepository interface {
	Migrate(ctx context.Context) error
	Create(ctx context.Context, message model.RainfallMessage) (*model.RainfallMessage, error)
}

func (r *RainfallRepo) Migrate(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS mqtt_message(
		id SERIAL PRIMARY KEY,
		topic TEXT NOT NULL,
		value INT NOT NULL
	);
	`

	_, err := r.db.ExecContext(ctx, query)
	return err
}

func (r *RainfallRepo) Create(ctx context.Context, msg model.RainfallMessage) (*model.RainfallMessage, error) {
	fmt.Println(msg)
	return &msg, nil
	// var id int64
	// err := r.db.QueryRowContext(ctx, "INSERT INTO websites(name, url, rank) values($1, $2, $3) RETURNING id", website.Name, website.URL, website.Rank).Scan(&id)
	// if err != nil {
	// 	var pgxError *pgconn.PgError
	// 	if errors.As(err, &pgxError) {
	// 		if pgxError.Code == "23505" {
	// 			return nil, ErrDuplicate
	// 		}
	// 	}
	// 	return nil, err
	// }
	// website.ID = id

	// return &website, nil
}
