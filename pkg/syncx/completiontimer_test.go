package syncx

import (
	"fmt"
	"testing"
	"time"
)

type checker struct {
	startTime    time.Time
	okCheckDelay time.Duration
}

func NewChecker(okCheckDelay time.Duration) *checker {
	return &checker{
		startTime:    time.Now(),
		okCheckDelay: okCheckDelay,
	}
}

func (c *checker) check() bool {
	fmt.Printf("do %v %v\n", time.Since(c.startTime) > c.okCheckDelay, c.okCheckDelay)
	return time.Since(c.startTime) > c.okCheckDelay
}

func Test_completionTimer_WaitForCompletion(t1 *testing.T) {
	tests := []struct {
		name           string
		maxIdleTime    time.Duration
		maxTotalTime   time.Duration
		okCheckDelay   time.Duration
		notifyInterval time.Duration
		wantErr        error
		minRunTime     time.Duration
		maxRunTime     time.Duration
	}{
		{
			"OK 1",
			100 * time.Millisecond,
			500 * time.Millisecond,
			0,
			100 * time.Millisecond,
			nil,
			0,
			2 * time.Millisecond,
		},
		{
			"OK 2",
			100 * time.Millisecond,
			5000 * time.Millisecond,
			500 * time.Millisecond,
			90 * time.Millisecond,
			nil,
			540 * time.Millisecond,
			542 * time.Millisecond,
		},
		{
			"OK 3",
			1000 * time.Millisecond,
			5000 * time.Millisecond,
			500 * time.Millisecond,
			100 * time.Millisecond,
			nil,
			500 * time.Millisecond,
			502 * time.Millisecond,
		},
		{
			"Idle timeout 1",
			100 * time.Millisecond,
			500 * time.Millisecond,
			200 * time.Millisecond,
			150 * time.Millisecond,
			IdleTimeout,
			100 * time.Millisecond,
			102 * time.Millisecond,
		},
		{
			"Idle timeout 2",
			100 * time.Millisecond,
			500 * time.Millisecond,
			2000 * time.Millisecond,
			150 * time.Millisecond,
			IdleTimeout,
			100 * time.Millisecond,
			102 * time.Millisecond,
		},
		{
			"Max time 1",
			100 * time.Millisecond,
			500 * time.Millisecond,
			1000 * time.Millisecond,
			10 * time.Millisecond,
			ExceededMaxTime,
			500 * time.Millisecond,
			502 * time.Millisecond,
		},
		{
			"Max time 2",
			600 * time.Millisecond,
			500 * time.Millisecond,
			10000 * time.Millisecond,
			1000 * time.Millisecond,
			ExceededMaxTime,
			500 * time.Millisecond,
			502 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := NewCompletionTimer(tt.maxIdleTime, tt.maxTotalTime, NewChecker(tt.okCheckDelay).check)

			tc := time.NewTicker(tt.notifyInterval)
			stop := make(chan bool)
			go func() {
				for {
					select {
					case xx := <-tc.C:
						fmt.Printf("Notify %v\n", xx)
						t.Notify()
					case <-stop:
						return
					}
				}
			}()

			start := time.Now()
			if err := t.WaitForCompletion(); err != tt.wantErr {
				t1.Errorf("WaitForCompletion() error = %v, wantErr %v", err, tt.wantErr)
			}
			duration := time.Since(start)
			fmt.Printf("Time: %v\n", duration)

			if duration < tt.minRunTime {
				t1.Errorf("WaitForCompletion() run time to short = %v, wantMinimum %v", duration, tt.minRunTime)
			}
			if duration > tt.maxRunTime {
				t1.Errorf("WaitForCompletion() run time to long = %v, wantMaximum %v", duration, tt.maxRunTime)
			}

			tc.Stop()
			close(stop)
		})
	}
}
