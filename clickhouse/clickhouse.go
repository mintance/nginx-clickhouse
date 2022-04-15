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
	"math/rand"
	"strings"
	"sync"
	"time"
)

type Storage struct {
	connActive      map[string]clickhouse.Conn
	connAll         map[string]clickhouse.Conn
	connActiveMutex sync.RWMutex
	Config          *config.Config
	ctx             context.Context
}

func NewStorage(config *config.Config, ctx context.Context) (*Storage, error) {
	var err error
	storage := Storage{
		connAll:    make(map[string]clickhouse.Conn),
		connActive: make(map[string]clickhouse.Conn),
		Config:     config,
		ctx:        ctx,
	}

	for _, host := range config.ClickHouse.Hosts {
		opts := &clickhouse.Options{
			Addr: []string{host},
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

		storage.connAll[host], err = clickhouse.Open(opts)
		if err != nil {
			return nil, err
		}
	}
	storage.check()
	go storage.checker()

	return &storage, nil
}

func (s *Storage) check() {
	for name, conn := range s.connAll {
		if err := conn.Ping(s.ctx); err != nil {
			s.connActiveMutex.Lock()
			delete(s.connActive, name)
			s.connActiveMutex.Unlock()
		} else {
			s.connActiveMutex.Lock()
			s.connActive[name] = conn
			s.connActiveMutex.Unlock()
		}
	}
}

func (s *Storage) checker() {
	for {
		s.check()
		time.Sleep(time.Second * 5)
	}
}

func (s *Storage) GetConn() clickhouse.Conn {
	s.connActiveMutex.RLock()
	defer s.connActiveMutex.RUnlock()
	if len(s.connActive) == 0 {
		logrus.Warn("No active CH connections in the pool")
		for _, conn := range s.connAll {
			return conn
		}
	}
	keys := make([]string, 0, len(s.connActive))
	for key, _ := range s.connActive {
		keys = append(keys, key)
	}
	key := keys[rand.Intn(len(keys))]
	return s.connActive[key]
}

func (s *Storage) Save(logs []*gonx.Entry) error {
	fields := getColumns(s.Config.ClickHouse.Columns)
	req := fmt.Sprintf("INSERT INTO %s (%s)", s.Config.ClickHouse.Table, strings.Join(fields, ","))
	batch, err := s.GetConn().PrepareBatch(s.ctx, req)
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
