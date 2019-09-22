package internal

import (
	"time"

	//_ "github.com/jackc/pgx/v4"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	//_ "github.com/lib/pq" // here
)

type DB struct{ db *sqlx.DB }

func Init(link string, maxOpen, maxIdle int, ttl time.Duration) (*DB, error) {
	db, err := sqlx.Connect("postgres", link)

	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(ttl)

	return &DB{db}, nil
}

func (db *DB) Close() error {
	return db.Close()
}

// CreateRequest add requesat to database
func (db *DB) CreateRequest(rdb *RequestDB) error {
	rdb.MakeHeaderRAW()
	sqlInsert := `
	INSERT INTO Request(method, address, header, body,
		userlogin, userpassword) VALUES
		(:method, :address, :header, :body,
			:userlogin, :userpassword)
			RETURNING *;
		`
	//_, err := db.db.Exec(sqlInsert, rdb)
	/*
			row := db.db.QueryRowx(sqlInsert)
		err := row.Scan(rdb)
	*/
	return db.createAndReturnStruct(sqlInsert, rdb)
}

func (db *DB) createAndReturnStruct(statement string, obj interface{}) error {
	rows, err := db.db.NamedQuery(statement, obj)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		err = rows.StructScan(obj)
	}
	return err
}

func (db *DB) GetRequests() (*RequestsDB, error) {

	statement := `select * from Request`
	rows, err := db.db.Queryx(statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	requests := make([]RequestDB, 0)

	for rows.Next() {
		var request RequestDB
		err = rows.StructScan(request)
		if err != nil {
			break
		}
		requests = append(requests, request)
	}
	if err != nil {
		return nil, err
	}
	requestsDB := &RequestsDB{
		Requests: requests,
	}
	return requestsDB, err
}

func (db *DB) GetRequest(id int) (*RequestDB, error) {
	statement := `select * from Request where id = $1`
	row := db.db.QueryRowx(statement, id)
	requestDB := &RequestDB{}
	err := row.StructScan(requestDB)
	return requestDB, err
}
