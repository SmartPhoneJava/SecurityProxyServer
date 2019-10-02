package database

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	//_ "github.com/jackc/pgx/v4"
	"github.com/SmartPhoneJava/SecurityProxyServer/internal/models"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type DB struct{ db *sqlx.DB }

type Settings struct {
	User     string
	Password string
	Addr     string
	Port     string
	Db       string
	MaxOpen  int
	MaxIdle  int
	TTL      time.Duration
}

func Init(settings Settings) (*DB, error) {
	link := "postgres://" + settings.User + ":" + settings.Password + "@" +
		settings.Addr + settings.Port + "/" + settings.Db + "?sslmode=disable"
	db, err := sqlx.Connect("postgres", link)

	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(settings.MaxOpen)
	db.SetMaxIdleConns(settings.MaxIdle)
	db.SetConnMaxLifetime(settings.TTL)

	return &DB{db}, nil
}

func (db *DB) Close() error {
	return db.Close()
}

// CreateRequest add requesat to database
func (db *DB) CreateRequest(rdb *models.RequestDB) error {
	rdb.MakeHeaderRAW()

	fmt.Println("method:", rdb.Method)
	fmt.Println("scheme:", rdb.Scheme)
	fmt.Println("address:", rdb.RemoteAddr)
	fmt.Println("header:", rdb.Header)
	fmt.Println("body:", rdb.Body)
	fmt.Println("userlogin:", rdb.UserLogin)
	fmt.Println("userpassword:", rdb.UserPassword)

	sqlInsert := `
	INSERT INTO Request(method, scheme, address, header, body,
		userlogin, userpassword) VALUES
		(:method, :scheme, :address, :header, :body,
			:userlogin, :userpassword)
			RETURNING *;
		`
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

func applyParameter(statement *string, counter *int, parameterInsert string, valid func() bool) {
	if valid() {
		if *counter > 0 {
			*statement += ` and `
		} else {
			*statement += ` where `
		}
		*statement += parameterInsert
		*counter++
	}
}

func addToQuery(key, value string) string {
	return " " + key + " = '" + value + "' "
}

func isInside(origin string, variants ...string) bool {
	for _, variant := range variants {
		if origin == variant {
			return true
		}
	}
	return false
}

func applyScheme(statement *string, counter *int, scheme string) {
	if strings.Contains(scheme, "'") || strings.Contains(scheme, ";") {
		return
	}
	applyParameter(statement, counter, addToQuery("scheme", scheme), func() bool {
		return scheme == "http" || scheme == "https"
	})
}

func applyMethod(statement *string, counter *int, method string) {
	if strings.Contains(method, "'") || strings.Contains(method, ";") {
		return
	}
	applyParameter(statement, counter, addToQuery("method", strings.ToUpper(method)), func() bool {
		var meth = strings.ToLower(method)
		return isInside(meth, "connect", "post", "get", "put", "options", "delete", "head")
	})
}

func applyAddress(statement *string, counter *int, address string) {
	if strings.Contains(address, "'") || strings.Contains(address, ";") {
		return
	}
	applyParameter(statement, counter, "POSITION (lower('"+address+"') IN lower(address)) > 0", func() bool {
		return address != ""
	})
}

func applyLimit(statement *string, limit string) {
	if _, err := strconv.Atoi(limit); err != nil {
		limit = "20"
	}
	*statement += " limit " + limit + " "
}

func applyDesc(statement *string, last string) {
	if last == "false" || last == "0" || last == "-" {
		return
	}
	*statement += " order by id desc "
}

func (db *DB) GetRequests(scheme, method, limit, last, address string) (*models.RequestsDB, error) {

	var (
		statement = `select * from Request`
		rows      *sqlx.Rows
		err       error
	)

	var counter = 0
	applyScheme(&statement, &counter, scheme)
	applyMethod(&statement, &counter, method)
	applyAddress(&statement, &counter, address)
	applyDesc(&statement, last)
	applyLimit(&statement, limit)

	fmt.Println("statement:", statement)

	rows, err = db.db.Queryx(statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	requests := make([]models.RequestDB, 0)

	for rows.Next() {
		var request models.RequestDB
		err = rows.StructScan(&request)
		if err != nil {
			break
		}
		requests = append(requests, request)
	}
	if err != nil {
		return nil, err
	}
	requestsDB := &models.RequestsDB{
		Requests: requests,
	}
	return requestsDB, err
}

func (db *DB) GetRequest(id int32) (*models.RequestDB, error) {
	statement := `select * from Request where id = $1`
	row := db.db.QueryRowx(statement, id)
	requestDB := &models.RequestDB{}
	err := row.StructScan(requestDB)
	return requestDB, err
}

func (db *DB) DeleteRequests() error {
	statement := `delete from Request`
	_, err := db.db.Exec(statement)
	return err
}
