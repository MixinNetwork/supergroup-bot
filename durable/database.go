package durable

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/MixinNetwork/supergroup/config"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type Database struct {
	pool *pgxpool.Pool
}

func NewDatabase(ctx context.Context) *Database {
	db := config.Config.Database
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", db.User, db.Password, db.Host, db.Port, db.Name)
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		log.Panicln(err)
	}
	config.MinConns = 6
	config.MaxConns = 256

	pool, err := pgxpool.ConnectConfig(context.Background(), config)
	if err != nil {
		log.Panicln(err)
	}
	err = pool.Ping(ctx)
	if err != nil {
		log.Panicln(err)
	}
	return &Database{pool: pool}
}

func (d *Database) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	return d.pool.Exec(ctx, sql, arguments...)
}

func (d *Database) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return d.pool.QueryRow(ctx, sql, args)
}

func (d *Database) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return d.pool.Query(ctx, sql, args)
}

func (d *Database) RunInTransaction(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error {
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	if err := fn(ctx, tx); err != nil {
		return tx.Rollback(ctx)
	}
	return tx.Commit(ctx)
}

func InsertQuery(table, args string) string {
	str := fmt.Sprintf("INSERT INTO %s(%s) ", table, args)
	length := len(strings.Split(args, ","))
	str += "VALUES("
	for i := 1; i <= length; i++ {
		if i == length {
			str += fmt.Sprintf("$%d)", i)
		} else {
			str += fmt.Sprintf("$%d,", i)
		}
	}
	return str
}

func InsertQueryOrUpdate(table, key, args string) string {
	str := ""
	if args == "" {
		str = InsertQuery(table, key)
		str += fmt.Sprintf(" ON CONFLICT(%s) DO NOTHING", key)
	} else {
		str = InsertQuery(table, fmt.Sprintf("%s,%s", key, args))
		keyLength := len(strings.Split(key, ",")) + 1
		argsArr := strings.Split(args, ",")
		str += fmt.Sprintf(" ON CONFLICT(%s) DO UPDATE SET ", key)
		length := len(argsArr)
		for i, s := range argsArr {
			str += fmt.Sprintf("%s=$%d", s, i+keyLength)
			if i != length-1 {
				str += ", "
			}
		}
	}
	return str
}

func InOperation(args []string) string {
	str := "("
	length := len(args)
	for i, s := range args {
		str += fmt.Sprintf("'%s'", s)
		if i < length-1 {
			str += ","
		}
	}
	str += ")"
	return str
}

type Row interface {
	Scan(dest ...interface{}) error
}

func CheckEmptyError(err error) error {
	if err == nil || IsEmpty(err) {
		return nil
	}
	return err
}

func CheckIsPKRepeatError(err error) bool {
	return strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}

func IsEmpty(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
