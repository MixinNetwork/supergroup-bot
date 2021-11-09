package durable

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/MixinNetwork/supergroup/config"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type Database struct {
	*pgxpool.Pool
}

func NewDatabase(ctx context.Context) *Database {
	connStr := ""
	if config.Config.Database.Port == "" {
		connStr = fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", config.Config.Database.User, config.Config.Database.Password, config.Config.Database.Host, config.Config.Database.Name)
	} else {
		connStr = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", config.Config.Database.User, config.Config.Database.Password, config.Config.Database.Host, config.Config.Database.Port, config.Config.Database.Name)
	}
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		panic(err)
	}
	config.MinConns = 4
	config.MaxConns = 256
	pool, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	return &Database{pool}
}

func (d *Database) ConnExec(ctx context.Context, sql string, arguments ...interface{}) error {
	conn, err := d.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	_, err = conn.Exec(ctx, sql, arguments...)
	return err
}

func (d *Database) ConnQuery(ctx context.Context, sql string, fn func(rows pgx.Rows) error, args ...interface{}) error {
	conn, err := d.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	err = fn(rows)
	if err != nil {
		return err
	}

	return nil
}

func (d *Database) RunInTransaction(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error {
	tx, err := d.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
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
