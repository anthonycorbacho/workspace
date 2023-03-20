package dlocksql

import (
	"context"
	"database/sql"
	"hash/fnv"
	"sync"

	dlock "github.com/anthonycorbacho/workspace/kit/distributedlock"
	"github.com/anthonycorbacho/workspace/kit/errors"
	kitsql "github.com/anthonycorbacho/workspace/kit/sql"
	"github.com/jmoiron/sqlx"
)

var _ dlock.DistributedLock = (*DistributedLock)(nil)
var _ dlock.Lock = (*Lock)(nil)

type DistributedLock struct {
	db *sqlx.DB
}

// NewDistributedLock return a new DistributedLock.
// A successful lock will hold a connection and a tx. So we want to be split from the
// applicatif connection pool
func NewDistributedLock(ctx context.Context, connection string, opts ...kitsql.Option) (*DistributedLock, error) {
	db, err := kitsql.Open(connection, opts...)
	if err != nil {
		return nil, err
	}
	if err := kitsql.StatusCheck(ctx, db); err != nil {
		return nil, err
	}
	return &DistributedLock{db: db}, nil
}

func (dl *DistributedLock) New(value string) (dlock.Lock, error) {
	hashedValue, err := hash(value)
	if err != nil {
		return nil, err
	}
	return &Lock{value: hashedValue, mutex: &sync.Mutex{}, db: dl.db}, nil
}

func hash(in string) (int64, error) {
	// for now we ignore the risk of collision because FNV-1a (64 bits) has low rate
	// of collision for similar string, and number of distributed lock will be relatively small
	// Future optimization could be to have a table string -> int64 in database that ensure
	// uniqueness of a int64
	hashed := fnv.New64a()
	_, err := hashed.Write([]byte(in))
	if err != nil {
		return 0, errors.Wrapf(err, "failed to hash value %s", in)
	}
	return int64(hashed.Sum64()), nil
}

type dblock struct {
	Succeed bool `db:"succeed"`
}

type Lock struct {
	db *sqlx.DB
	// current transaction that holds the lock
	// nil if not locked
	tx    *sqlx.Tx
	value int64
	// mutex to protect concurrent tx set
	mutex *sync.Mutex
}

// IsLock implements dlock.Lock
func (l *Lock) IsLock(ctx context.Context) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.isLock(ctx)
}

func (l *Lock) Release() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.release()
}

func (l *Lock) Lock(ctx context.Context) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.tx == nil {
		tx, err := l.db.BeginTxx(ctx, &sql.TxOptions{})
		if err != nil {
			return err
		}

		res := dblock{}
		const q = `SELECT pg_try_advisory_xact_lock($1) as succeed;`
		err = tx.GetContext(ctx, &res, q, l.value)
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				return errors.Wrap(err, rollbackErr.Error())
			}
			return errors.Wrap(err, "failed to get lock status")
		}
		if !res.Succeed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				return errors.Wrap(err, rollbackErr.Error())
			}
			return dlock.ErrAcquiredLock
		}
		l.tx = tx
		return nil
	}

	return l.isLock(ctx)
}

// release should be call after mutex has been acquired
func (l *Lock) release() error {
	if l.tx != nil {
		err := l.tx.Rollback()
		l.tx = nil
		return err
	}
	return nil
}

// isLock should be call after mutex has been acquired
func (l *Lock) isLock(ctx context.Context) error {
	if l.tx == nil {
		return errors.Wrap(dlock.ErrAcquiredLock, "does not lock the ressource")
	}
	res := dblock{}
	// We do not know to query any information of the lock, posgressql spec
	// ensure that if the TX is valid the lock is still ours
	err := l.tx.GetContext(ctx, &res, "SELECT TRUE as succeed")
	if err != nil {
		if err := l.release(); err != nil {
			return errors.Wrap(dlock.ErrAcquiredLock, err.Error())
		}
		return dlock.ErrAcquiredLock
	}
	return nil
}
