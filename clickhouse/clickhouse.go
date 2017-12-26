package clickhouse

import (
	"github.com/mintance/go-clickhouse"
	"github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
	"github.com/satyrius/gonx"
	"net/url"
	"reflect"
	"github.com/Sirupsen/logrus"
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

func buildRows(keys []string, columns map[string]string, data []gonx.Entry) (rows clickhouse.Rows) {

	for _, logEntry := range data {
		row := clickhouse.Row{}

		for _, column := range keys {
			value, err := logEntry.Field(columns[column])
			if err != nil {
				logrus.Errorf("error to build rows: %v", err)
			}
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

	cHTTP := clickhouse.NewHttpTransport()
	conn := clickhouse.NewConn(config.ClickHouse.Host+":"+config.ClickHouse.Port, cHTTP)

	params := url.Values{}
	params.Add("user", config.ClickHouse.Credentials.User)
	params.Add("password", config.ClickHouse.Credentials.Password)
	conn.SetParams(params)

	if err := conn.Ping(); err != nil {
		return nil, err
	}

	return conn, nil
}
