package main

import (
	"github.com/satyrius/gonx"
	"net/url"
	"github.com/mintance/go-clickhouse"
	"reflect"
)

var clickhouse_storage *clickhouse.Conn

func save(config *Config, logs []gonx.Entry) error {

	storage, err := getStorage(config)

	if err != nil {
		return err
	}

	query, err := clickhouse.BuildMultiInsert(
		config.ClickHouse.Db + "." + config.ClickHouse.Table,
		getColumns(config.ClickHouse.Columns),
		buildRows(config.ClickHouse.Columns, logs),
	)

	if err != nil {
		return err
	}

	return query.Exec(storage)
}

func getColumns(columns map[string]string) []string {

	keys := reflect.ValueOf(columns).MapKeys()

	string_columns := make([]string, len(keys))

	for i := 0; i < len(keys); i++ {
		string_columns[i] = keys[i].String()
	}

	return string_columns
}

func buildRows(columns map[string]string, data []gonx.Entry) clickhouse.Rows {

	rows := []clickhouse.Row{}

	for _, log_entry := range data {

		row := clickhouse.Row{}

		for _, key := range columns {

			value, _ := log_entry.Field(key)

			row = append(row, value)
		}

		rows = append(rows, row)
	}

	return rows
}

func getStorage(config *Config) (*clickhouse.Conn, error) {

	if clickhouse_storage != nil {
		return clickhouse_storage, nil
	}

	chttp := clickhouse.NewHttpTransport()

	conn := clickhouse.NewConn(config.ClickHouse.Host, chttp)

	params := url.Values{}

	params.Add("user", config.ClickHouse.Credentials.User)
	params.Add("password", config.ClickHouse.Credentials.Password)

	conn.SetParams(params)

	err := conn.Ping()

	if err != nil {
		return nil, err
	}

	return conn, nil
}