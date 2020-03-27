package syncx

import (
	"errors"
	log "github.com/sirupsen/logrus"
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
	doneChan        chan bool
	started         bool
}

func NewCompletionTimer(maxIdleTime, maxTotalTime time.Duration, check func() bool) *CompletionTimer {
	log.Tracef("MaxIdleTime: %v, MaxTotalTime: %v", maxIdleTime, maxTotalTime)
	return &CompletionTimer{
		maxIdleTime:  maxIdleTime,
		maxTotalTime: maxTotalTime,
		check:        check,
		doneChan:     make(chan bool),
	}
}

func (t *CompletionTimer) Notify() {
	if !t.started {
		return
	}

	t.lastCheckResult = t.check()
	if t.lastCheckResult {
		t.doneChan <- true
	}
	if t.idleTimer != nil && !t.idleTimer.Stop() {
		<-t.idleTimer.C
	}
	t.idleTimer = time.NewTimer(t.maxIdleTime)
}

func (t *CompletionTimer) WaitForCompletion() (err error) {
	t.lastCheckResult = t.check()
	if t.lastCheckResult {
		return
	}
	t.waitTimer = time.NewTimer(t.maxTotalTime)
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
				return IdleTimeout
			}
		case ok := <-t.doneChan:
			if ok {
				return
			} else {
				return Cancelled
			}
		}
	}
}

func (t *CompletionTimer) Cancel() {
	t.doneChan <- false
}
