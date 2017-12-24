package clickhouse

import (
	"github.com/mintance/go-clickhouse"
	"github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
	"github.com/satyrius/gonx"
	"net/url"
	"reflect"
)

var clickHouseStorage *clickhouse.Conn

func Save(config *config.Config, logs []gonx.Entry) error {

	storage, err := getStorage(config)

	if err != nil {
		return err
	}

	columns := getColumns(config.ClickHouse.Columns)

	rows := buildRows(columns, config.ClickHouse.Columns, logs)

	query, err := clickhouse.BuildMultiInsert(
		config.ClickHouse.Db+"."+config.ClickHouse.Table,
		columns,
		rows,
	)

	if err != nil {
		return err
	}

	return query.Exec(storage)
}

func getColumns(columns map[string]string) []string {

	keys := reflect.ValueOf(columns).MapKeys()
	stringColumns := make([]string, len(keys))

	for i := 0; i < len(keys); i++ {
		stringColumns[i] = keys[i].String()
	}

	return stringColumns
}

func buildRows(keys []string, columns map[string]string, data []gonx.Entry) clickhouse.Rows {

	var rows []clickhouse.Row

	for _, logEntry := range data {
		row := clickhouse.Row{}

		for _, column := range keys {
			value, _ := logEntry.Field(columns[column])
			row = append(row, nginx.ParseField(columns[column], value))
		}

		rows = append(rows, row)
	}

	return rows
}

func getStorage(config *config.Config) (*clickhouse.Conn, error) {

	if clickHouseStorage != nil {
		return clickHouseStorage, nil
	}

	cHttp := clickhouse.NewHttpTransport()
	conn := clickhouse.NewConn(config.ClickHouse.Host+":"+config.ClickHouse.Port, cHttp)

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
