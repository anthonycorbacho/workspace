package dlocksql

import (
	"context"
	"os"
	"testing"
	"time"

	dlock "github.com/anthonycorbacho/workspace/kit/distributedlock"
	kitsql "github.com/anthonycorbacho/workspace/kit/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type DlocksqlTestSuite struct {
	suite.Suite
	db      kitsql.TestingDB
	storage *DistributedLock
}

func TestDLocksqlTestSuite(t *testing.T) {
	suite.Run(t, new(DlocksqlTestSuite))
}

func (dts *DlocksqlTestSuite) SetupSuite() {
	if os.Getenv("TESTINGDB_URL") == "" {
		dts.T().Skip("Skipping, no testing database setup via env variable TESTINGDB_URL")
	}
	err := dts.db.Open()
	assert.NoError(dts.T(), err)
}

func (dts *DlocksqlTestSuite) TearDownSuite() {
	err := dts.db.Close()
	if err != nil {
		dts.Fail("should not expect error when closing testing db", err)
	}
}

func (dts *DlocksqlTestSuite) SetupTest() {
	db, err := kitsql.Open(dts.db.DSN)
	if err != nil {
		dts.Fail("should not expect error when setting up test", err)
	}
	dts.storage = &DistributedLock{db}
}

func (dts *DlocksqlTestSuite) TestDLock_Backfill() {
	// first we initialize all the lock we will use
	mylock, err := dts.storage.New("mylock")
	dts.Assert().Nil(err)
	myLock2, err := dts.storage.New("mylock")
	dts.Assert().Nil(err)
	differentlock, err := dts.storage.New("differentlock")
	dts.Assert().Nil(err)

	// should succeed to lock when no lock was made
	err = mylock.Lock(context.TODO())
	dts.Assert().Nil(err)
	// should fail to acquired the lock that was just made
	err = myLock2.Lock(context.TODO())
	dts.Assert().Error(dlock.ErrAcquiredLock, err)
	// should ensure we still have the lock on mylock
	err = mylock.Lock(context.TODO())
	dts.Assert().Nil(err)

	// should check that we can lock an other value
	err = differentlock.Lock(context.TODO())
	dts.Assert().Nil(err)

	// should check that we can release lock
	dts.Assert().Nil(mylock.Release())

	// we can check that myLock2 can now lock
	err = myLock2.Lock(context.TODO())
	dts.Assert().Nil(err)

	// should check that we can acquired again the lock
	dts.Assert().Nil(mylock.Release())
	dts.Assert().Nil(myLock2.Release())
	dts.Assert().Nil(differentlock.Release())
}

func (dts *DlocksqlTestSuite) TestDLock_WaitForLock() {
	mylock, err := dts.storage.New("mylock")
	dts.Assert().Nil(err)
	myLock2, err := dts.storage.New("mylock")
	dts.Assert().Nil(err)

	ctx := context.TODO()
	dts.Assert().Nil(dlock.WaitForLock(ctx, mylock))

	// should timeout for mylock2
	ctxDeadline, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	err = dlock.WaitForLock(ctxDeadline, myLock2)
	dts.Assert().ErrorContains(err, "timeout to acquired lock: context deadline exceeded")

	go func() {
		// we release mylock after 500ms
		time.Sleep(time.Millisecond * 500)
		err := mylock.Release()
		dts.Assert().Nil(err)
	}()
	ctxDeadline, cancel = context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	err = dlock.WaitForLock(ctxDeadline, myLock2)
	dts.Assert().Nil(err)
}

func (dts *DlocksqlTestSuite) TestDLock_ContextLock() {
	mylock, err := dts.storage.New("mylock")
	dts.Assert().Nil(err)

	ctx, cancel, err := dlock.ContextLock(context.TODO(), mylock)
	defer cancel()
	dts.Assert().Nil(err)

	go func() {
		// we release mylock after 2s
		time.Sleep(time.Second * 2)
		err := mylock.Release()
		dts.Assert().Nil(err)
	}()

	countDone := 0
	countNotDone := 0
	// maximum we wait 5sc
	for countDone+countNotDone < 5 {
		select {
		case <-ctx.Done():
			countDone++
		case <-time.After(1 * time.Second):
			countNotDone++
		}
	}
	// should at least be 2 because during 2 seconds the lock was not release
	dts.Assert().GreaterOrEqual(countNotDone, 2)
	// should at least be 2 because after 3 sc we are sure that the context was release
	dts.Assert().GreaterOrEqual(countDone, 2)
}
