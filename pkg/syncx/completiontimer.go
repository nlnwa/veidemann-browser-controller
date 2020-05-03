package syncx

import (
	"errors"
	log "github.com/sirupsen/logrus"
	"sync/atomic"
	"time"
)

var IdleTimeout = errors.New("idle timeout")
var ExceededMaxTime = errors.New("exceeded max time")
var Cancelled = errors.New("cancelled")

type CompletionTimer struct {
	maxIdleTime     time.Duration
	maxTotalTime    time.Duration
	check           func() bool
	lastCheckResult bool
	waitTimer       *time.Timer
	idleTimer       *time.Timer
	idleTimeout     time.Time
	doneChan        chan interface{}
	started         bool
	done            int32 // 0: not done, 1: success, 2: cancel
}

func NewCompletionTimer(maxIdleTime, maxTotalTime time.Duration, check func() bool) *CompletionTimer {
	log.Tracef("MaxIdleTime: %v, MaxTotalTime: %v", maxIdleTime, maxTotalTime)
	if check == nil {
		check = func() bool {
			return false
		}
	}
	return &CompletionTimer{
		maxIdleTime:  maxIdleTime,
		maxTotalTime: maxTotalTime,
		check:        check,
		doneChan:     make(chan interface{}),
	}
}

func (t *CompletionTimer) Notify() {
	if !t.started {
		return
	}

	t.lastCheckResult = t.check()
	if t.lastCheckResult {
		if atomic.CompareAndSwapInt32(&t.done, 0, 1) {
			close(t.doneChan)
			return
		}
	}
	if t.idleTimer != nil {
		t.idleTimeout = time.Now().Add(t.maxIdleTime)
	}
}

func (t *CompletionTimer) WaitForCompletion() (err error) {
	t.lastCheckResult = t.check()
	if t.lastCheckResult {
		return
	}
	t.waitTimer = time.NewTimer(t.maxTotalTime)
	t.idleTimeout = time.Now().Add(t.maxIdleTime)
	t.idleTimer = time.NewTimer(t.maxIdleTime)
	t.started = true

	for {
		select {
		case <-t.waitTimer.C:
			t.lastCheckResult = t.check()
			if t.lastCheckResult {
				return
			} else {
				return ExceededMaxTime
			}
		case <-t.idleTimer.C:
			t.lastCheckResult = t.check()
			if t.lastCheckResult {
				return
			} else {
				if time.Now().After(t.idleTimeout) {
					return IdleTimeout
				} else {
					t.idleTimer = time.NewTimer(t.idleTimeout.Sub(time.Now()))
				}
			}
		case <-t.doneChan:
			if atomic.LoadInt32(&t.done) == 1 {
				return
			} else {
				return Cancelled
			}
		}
	}
}

func (t *CompletionTimer) Cancel() {
	if atomic.CompareAndSwapInt32(&t.done, 0, 2) {
		close(t.doneChan)
	}
}
