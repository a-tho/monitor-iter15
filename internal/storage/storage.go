// Package storage implements a trivial storage.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"

	monitor "github.com/a-tho/monitor/internal"
	"github.com/a-tho/monitor/internal/retry"
)

// MemStorage represents the storage.
type MemStorage struct {
	// Database
	db                *sqlx.DB
	stmtSetGauge      *sqlx.Stmt
	stmtAddCounter    *sqlx.Stmt
	stmtGetGauge      *sqlx.Stmt
	stmtGetCounter    *sqlx.Stmt
	stmtStringGauge   *sqlx.Stmt
	stmtStringCounter *sqlx.Stmt
	stmtAllGauge      *sqlx.Stmt
	stmtAllCounter    *sqlx.Stmt

	// Memory
	DataGauge   map[string]monitor.Gauge
	DataCounter map[string]monitor.Counter
	file        *os.File
	m           sync.Mutex
	syncMode    bool // Whether recording is synchronuous
}

// New returns an initialized storage.
func New(ctx context.Context, dsn string, fileStoragePath string, storeInterval int, restore bool) (*MemStorage, error) {
	if dsn != "" {
		// DB may be available
		storage, err := NewDBStorage(ctx, dsn)
		if err == nil {
			return storage, nil
		}
		log.Err(err).Msg("Failed to init DB storage. Now trying to init memory storage...")
		// DB not available, revert to memory storage
	}

	// No available database, store in memory
	return NewMemStorage(ctx, fileStoragePath, storeInterval, restore)
}

func NewDBStorage(ctx context.Context, dsn string) (*MemStorage, error) {
	storage := MemStorage{}

	err := retry.Do(ctx, func(context.Context) error {
		db, err := sqlx.Open("pgx", dsn)
		if err != nil {
			return retry.RetriableError(err)
		}

		_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS gauge (
			"name" VARCHAR(50) PRIMARY KEY,
			"value" DOUBLE PRECISION
		);`)
		if err != nil {
			db.Close()
			return retry.RetriableError(err)
		}

		_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS counter (
			"name" VARCHAR(50) PRIMARY KEY,
			"value" NUMERIC
		);`)
		if err != nil {
			db.Close()
			return retry.RetriableError(err)
		}

		stmtSetGauge, err := db.Preparex(`
		INSERT INTO gauge (name, value)
		VALUES
			($1, $2)
		ON CONFLICT (name) DO UPDATE
		SET value = EXCLUDED.value;`)
		if err != nil {
			db.Close()
			return retry.RetriableError(err)
		}

		stmtAddCounter, err := db.Preparex(`
		INSERT INTO counter (name, value)
		VALUES
			($1, $2)
		ON CONFLICT (name) DO UPDATE
		SET value = counter.value + EXCLUDED.value;`)
		if err != nil {
			db.Close()
			return retry.RetriableError(err)
		}

		stmtGetGauge, err := db.Preparex(`
		SELECT value FROM gauge WHERE name = $1`)
		if err != nil {
			db.Close()
			return retry.RetriableError(err)
		}

		stmtGetCounter, err := db.Preparex(`
		SELECT value FROM counter WHERE name = $1`)
		if err != nil {
			db.Close()
			return retry.RetriableError(err)
		}

		// https://alphahydrae.com/2021/02/how-to-export-postgresql-data-to-a-json-file/
		stmtStringGauge, err := db.Preparex(`
		SELECT json_agg(row_to_json(gauge)) FROM gauge;`)
		if err != nil {
			db.Close()
			return retry.RetriableError(err)
		}

		stmtStringCounter, err := db.Preparex(`
		SELECT json_agg(row_to_json(counter)) FROM counter;`)
		if err != nil {
			db.Close()
			return retry.RetriableError(err)
		}

		stmtAllGauge, err := db.Preparex(`
		SELECT name, value FROM gauge`)
		if err != nil {
			db.Close()
			return retry.RetriableError(err)
		}

		stmtAllCounter, err := db.Preparex(`
		SELECT name, value FROM counter`)
		if err != nil {
			db.Close()
			return retry.RetriableError(err)
		}

		storage.db = db
		storage.stmtSetGauge = stmtSetGauge
		storage.stmtAddCounter = stmtAddCounter
		storage.stmtGetGauge = stmtGetGauge
		storage.stmtGetCounter = stmtGetCounter
		storage.stmtStringGauge = stmtStringGauge
		storage.stmtStringCounter = stmtStringCounter
		storage.stmtAllGauge = stmtAllGauge
		storage.stmtAllCounter = stmtAllCounter

		return nil
	})
	if err != nil {
		log.Info().Msg("Failed to init DB storage")

		return nil, err
	}

	log.Info().Msg("Initialized DB storage successfully")

	return &storage, nil
}

func NewMemStorage(ctx context.Context, fileStoragePath string, storeInterval int, restore bool) (*MemStorage, error) {
	storage := MemStorage{
		DataGauge:   make(map[string]monitor.Gauge),
		DataCounter: make(map[string]monitor.Counter),
	}

	if fileStoragePath != "" {
		var file *os.File

		err := retry.Do(ctx, func(ctx context.Context) (err error) {
			file, err = os.OpenFile(fileStoragePath, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o644)
			if err != nil {
				return retry.RetriableError(err)
			}
			return nil
		})
		if err != nil {
			return &storage, nil
		}

		if restore {
			dec := json.NewDecoder(file)
			var storageIn MemStorage
			if err = dec.Decode(&storageIn); err == nil {
				storage.DataGauge = storageIn.DataGauge
				storage.DataCounter = storageIn.DataCounter
			}
		}

		syncMode := storeInterval == 0
		storage.file = file
		storage.syncMode = syncMode

		if !syncMode {
			go storage.memBackup(storeInterval)
		}
	}

	log.Info().Msg("Initialized memory storage successfully")

	return &storage, nil
}

func (s *MemStorage) memBackup(storeInterval int) {
	// Write to the file every storeInterval seconds
	var ticker <-chan time.Time
	if storeInterval > 0 {
		t := time.NewTicker(time.Duration(storeInterval) * time.Second)
		defer t.Stop()
		ticker = t.C
	}

	// and close the file when SIGINT is passed
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT)
	signal.Notify(quit, syscall.SIGQUIT)

	for {
		select {
		case <-ticker:
			s.writeToFile()
		case <-quit:
			return
		}
	}
}

// SetGauge inserts or updates a gauge metric value v for the key k.
func (s *MemStorage) SetGauge(ctx context.Context, k string, v monitor.Gauge) (monitor.MetricRepo, error) {
	if s.db != nil {
		err := retry.Do(ctx, func(context.Context) error {
			_, err := s.stmtSetGauge.ExecContext(ctx, k, v)
			return s.retryIfPgConnException(err)
		})

		return s, err
	}

	// No DB, use memory
	s.m.Lock()
	s.DataGauge[k] = v
	s.m.Unlock()

	if s.syncMode {
		s.writeToFile()
	}

	return s, nil
}

// SetGaugeBatch inserts or updates a gauge metrics batch.
func (s *MemStorage) SetGaugeBatch(ctx context.Context, batch []*monitor.Metrics) (monitor.MetricRepo, error) {
	if s.db != nil {
		err := retry.Do(ctx, func(context.Context) error {
			tx, err := s.db.BeginTxx(ctx, nil)
			if err != nil {
				return s.retryIfPgConnException(err)
			}
			defer tx.Rollback()

			stmt := tx.StmtxContext(ctx, s.stmtSetGauge)
			defer stmt.Close()

			for _, metric := range batch {
				_, err = stmt.ExecContext(ctx, metric.ID, metric.Value)
				if err != nil {
					return s.retryIfPgConnException(err)
				}
			}
			err = tx.Commit()
			return s.retryIfPgConnException(err)
		})

		return s, err
	}

	// No DB, use memory
	s.m.Lock()
	for _, metric := range batch {
		s.DataGauge[metric.ID] = monitor.Gauge(*metric.Value) // won't be nil, checked for it earlier
	}
	s.m.Unlock()

	if s.syncMode {
		s.writeToFile()
	}

	return s, nil
}

// AddCounter adds a counter metric value v for the key k.
func (s *MemStorage) AddCounter(ctx context.Context, k string, v monitor.Counter) (monitor.MetricRepo, error) {
	if s.db != nil {
		err := retry.Do(ctx, func(context.Context) error {
			_, err := s.stmtAddCounter.ExecContext(ctx, k, v)
			return s.retryIfPgConnException(err)
		})

		return s, err
	}

	s.m.Lock()
	s.DataCounter[k] += v
	s.m.Unlock()

	if s.syncMode {
		s.writeToFile()
	}

	return s, nil
}

// AddCounterBatch adds a counter metrics batch.
func (s *MemStorage) AddCounterBatch(ctx context.Context, batch []*monitor.Metrics) (monitor.MetricRepo, error) {
	if s.db != nil {
		err := retry.Do(ctx, func(context.Context) error {
			tx, err := s.db.BeginTxx(ctx, nil)
			if err != nil {
				return s.retryIfPgConnException(err)
			}
			defer tx.Rollback()

			stmt := tx.StmtxContext(ctx, s.stmtAddCounter)
			defer stmt.Close()

			for _, metric := range batch {
				_, err = stmt.ExecContext(ctx, metric.ID, metric.Delta)
				if err != nil {
					return s.retryIfPgConnException(err)
				}
			}
			err = tx.Commit()
			return s.retryIfPgConnException(err)
		})

		return s, err
	}

	s.m.Lock()
	for _, metric := range batch {
		s.DataCounter[metric.ID] += monitor.Counter(*metric.Delta) // won't be nil, checked for it in the caller function
	}
	s.m.Unlock()

	if s.syncMode {
		s.writeToFile()
	}

	return s, nil
}

// GetGauge retrieves the gauge value for the key k.
func (s *MemStorage) GetGauge(ctx context.Context, k string) (v monitor.Gauge, ok bool) {
	if s.db != nil {
		err := retry.Do(ctx, func(context.Context) error {
			row := s.stmtGetGauge.QueryRow(k)
			err := row.Scan(&v)
			return s.retryIfPgConnException(err)
		})
		if err != nil {
			return v, false
		}
		return v, true
	}

	s.m.Lock()
	v, ok = s.DataGauge[k]
	s.m.Unlock()

	return
}

// GetCounter retrieves the counter value for the key k.
func (s *MemStorage) GetCounter(ctx context.Context, k string) (v monitor.Counter, ok bool) {
	if s.db != nil {
		err := retry.Do(ctx, func(context.Context) error {
			row := s.stmtGetCounter.QueryRowContext(ctx, k)
			err := row.Scan(&v)
			return s.retryIfPgConnException(err)
		})
		if err != nil {
			return v, false
		}
		return v, true
	}

	s.m.Lock()
	v, ok = s.DataCounter[k]
	s.m.Unlock()

	return
}

// StringGauge produces a JSON representation of gauge metrics kept in the
// storage
func (s *MemStorage) StringGauge(ctx context.Context) (string, error) {
	if s.db != nil {
		var enc string
		err := retry.Do(ctx, func(context.Context) error {
			row := s.stmtStringCounter.QueryRowContext(ctx)
			err := row.Scan(&enc)
			return s.retryIfPgConnException(err)
		})

		return enc, err
	}

	s.m.Lock()
	out, err := json.Marshal(s.DataGauge)
	s.m.Unlock()

	return string(out), err
}

// StringCounter produces a JSON representation of counter metrics kept in the
// storage
func (s *MemStorage) StringCounter(ctx context.Context) (string, error) {
	if s.db != nil {
		var enc string
		err := retry.Do(ctx, func(context.Context) error {
			row := s.stmtStringCounter.QueryRowContext(ctx)
			err := row.Scan(&enc)
			return s.retryIfPgConnException(err)
		})
		return enc, err
	}

	s.m.Lock()
	out, err := json.Marshal(s.DataCounter)
	s.m.Unlock()

	return string(out), err
}

// WriteAllGauge writes gauge metrics as HTML into specified writer.
func (s *MemStorage) WriteAllGauge(ctx context.Context, wr io.Writer) error {
	tmpl, err := template.New("metrics").Parse(metricsTemplate)
	if err != nil {
		return err
	}

	if s.db != nil {
		var (
			key   string
			value monitor.Gauge
		)
		dataGauge := make(map[string]monitor.Gauge)

		err := retry.Do(ctx, func(context.Context) error {
			rows, err := s.stmtAllGauge.QueryContext(ctx)
			if err != nil {
				return s.retryIfPgConnException(err)
			}
			defer rows.Close()

			for rows.Next() {
				if err = rows.Scan(&key, &value); err != nil {
					// TODO replace with clear(dataGauge) once Go 1.21 is out
					for key := range dataGauge {
						delete(dataGauge, key)
					}
					return s.retryIfPgConnException(err)
				}
				dataGauge[key] = value
			}
			if rows.Err() != nil {
				// TODO replace with clear(dataGauge) once Go 1.21 is out
				for key := range dataGauge {
					delete(dataGauge, key)
				}
				return s.retryIfPgConnException(err)
			}
			return nil
		})
		if err != nil {
			return err
		}

		err = tmpl.Execute(wr, dataGauge)

		return err
	}

	s.m.Lock()
	err = tmpl.Execute(wr, s.DataGauge)
	s.m.Unlock()

	return err
}

// WriteAllCounter writes counter metrics as HTML into specified writer.
func (s *MemStorage) WriteAllCounter(ctx context.Context, wr io.Writer) error {
	tmpl, err := template.New("metrics").Parse(metricsTemplate)
	if err != nil {
		return err
	}

	if s.db != nil {
		var (
			key   string
			value monitor.Counter
		)
		dataCounter := make(map[string]monitor.Counter)

		err := retry.Do(ctx, func(context.Context) error {
			rows, err := s.stmtAllCounter.QueryContext(ctx)
			if err != nil {
				return s.retryIfPgConnException(err)
			}
			defer rows.Close()

			for rows.Next() {
				if err = rows.Scan(&key, &value); err != nil {
					// TODO replace with clear(dataGauge) once Go 1.21 is out
					for key := range dataCounter {
						delete(dataCounter, key)
					}
					return s.retryIfPgConnException(err)
				}
				dataCounter[key] = value
			}
			if rows.Err() != nil {
				// TODO replace with clear(dataGauge) once Go 1.21 is out
				for key := range dataCounter {
					delete(dataCounter, key)
				}
				return s.retryIfPgConnException(err)
			}
			return nil
		})
		if err != nil {
			return err
		}

		err = tmpl.Execute(wr, dataCounter)

		return err
	}

	s.m.Lock()
	err = tmpl.Execute(wr, s.DataCounter)
	s.m.Unlock()

	return err
}

func (s *MemStorage) PingContext(ctx context.Context) error {
	err := retry.Do(ctx, func(context.Context) error {
		err := s.db.PingContext(ctx)
		return s.retryIfPgConnException(err)
	})

	return err
}

func (s *MemStorage) Close() error {
	if s.db != nil {
		s.stmtSetGauge.Close()
		s.stmtAddCounter.Close()
		s.stmtGetGauge.Close()
		s.stmtGetCounter.Close()
		s.stmtStringGauge.Close()
		s.stmtStringCounter.Close()
		return s.db.Close()
	}

	s.writeToFile()

	if s.file == nil {
		return nil
	}
	return s.file.Close()
}

func (s *MemStorage) writeToFile() (err error) {
	if s.file == nil {
		return nil
	}

	s.m.Lock()

	// no real context here because write to the file needs to happen
	// regardless of context canceling etc
	err = retry.Do(context.Background(), func(context.Context) error {
		if err = s.file.Truncate(0); err != nil {
			return retry.RetriableError(err)
		}
		enc := json.NewEncoder(s.file)
		if err = enc.Encode(s); err != nil {
			return retry.RetriableError(err)
		}
		return nil
	})

	s.m.Unlock()

	return err
}

func (s *MemStorage) retryIfPgConnException(err error) error {
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgerrcode.IsConnectionException(pgErr.Code) {
				return retry.RetriableError(err)
			}
		}
	}
	return err
}

const metricsTemplate = `
		{{range $key, $value := .}}
			<p>{{$key}}: {{$value}}</p>
		{{end}}`
