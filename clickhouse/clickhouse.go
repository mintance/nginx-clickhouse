package clickhouse

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/compress"
	"github.com/WinnerSoftLab/nginx-clickhouse/config"
	"github.com/WinnerSoftLab/nginx-clickhouse/nginx"
	"github.com/satyrius/gonx"
	"github.com/sirupsen/logrus"
	"strings"
	"time"
)

type Storage struct {
	Conn   clickhouse.Conn
	Config *config.Config
	ctx    context.Context
}

func NewStorage(config *config.Config, ctx context.Context) (*Storage, error) {
	var err error
	storage := Storage{
		Config: config,
		ctx:    ctx,
	}

	opts := &clickhouse.Options{
		Addr: config.ClickHouse.Hosts,
		Auth: clickhouse.Auth{
			Database: config.ClickHouse.Db,
			Username: config.ClickHouse.Credentials.User,
			Password: config.ClickHouse.Credentials.Password,
		},
		MaxOpenConns:     8,
		MaxIdleConns:     4,
		ConnMaxLifetime:  time.Hour,
		DialTimeout:      time.Second * 15,
		Compression:      &clickhouse.Compression{compress.ZSTD},
		ConnOpenStrategy: clickhouse.ConnOpenRoundRobin,
	}

	if config.ClickHouse.Secure {
		opts.TLS = &tls.Config{
			InsecureSkipVerify: config.ClickHouse.SkipVerify,
		}
	}

	storage.Conn, err = clickhouse.Open(opts)
	if err != nil {
		return nil, err
	}

	if err := storage.Conn.Ping(context.Background()); err != nil {
		return nil, err
	}

	return &storage, nil
}

func (s *Storage) Save(logs []*gonx.Entry) error {
	fields := getColumns(s.Config.ClickHouse.Columns)
	req := fmt.Sprintf("INSERT INTO %s (%s)", s.Config.ClickHouse.Table, strings.Join(fields, ","))
	batch, err := s.Conn.PrepareBatch(s.ctx, req)
	if err != nil {
		return err
	}

	rows := buildRows(fields, s.Config.ClickHouse.Columns, logs)

	for _, row := range rows {
		if err := batch.Append(row...); err != nil {
			return err
		}
	}

	if err := batch.Send(); err != nil {
		return err
	}

	return nil
}

func getColumns(columns map[string]string) []string {
	cols := make([]string, 0, len(columns))
	for k, _ := range columns {
		cols = append(cols, k)
	}
	return cols
}

func buildRows(fields []string, columns map[string]string, data []*gonx.Entry) (rows [][]interface{}) {
	rows = make([][]interface{}, 0, len(data))

Rows:
	for _, logEntry := range data {
		row := make([]interface{}, 0, len(columns))
		for _, v := range fields {
			if value, err := logEntry.Field(columns[v]); err != nil {
				logrus.Errorf("error to build rows: %v", err)
				continue Rows
			} else {
				row = append(row, nginx.ParseField(columns[v], value))
			}
		}
		rows = append(rows, row)
	}

	return rows
}
